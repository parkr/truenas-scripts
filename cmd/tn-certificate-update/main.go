package main

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/truenas/api_client_golang/truenas_api"
)

type CertificateCreatePayload struct {
	Name        string  `json:"name"`
	CreateType  string  `json:"create_type"`
	Certificate string  `json:"certificate"`
	PrivateKey  string  `json:"privatekey"`
	Passphrase  *string `json:"passphrase"`
}

type CertificateUpdatePayload struct {
	Name string `json:"name,omitempty"`
}

type Certificate struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Certificate string `json:"certificate"`
}

type SystemGeneralConfig struct {
	UICertificate int64 `json:"ui_certificate"`
}

type SystemGeneralUpdatePayload struct {
	UICertificate int64 `json:"ui_certificate"`
}

type RPCResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *RPCError       `json:"error"`
	ID     interface{}     `json:"id"`
}

type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

type TNClient interface {
	Login(user, pass, token string) error
	Call(method string, timeout int64, params interface{}) (json.RawMessage, error)
	Close() error
}

type JobInfo struct {
	ID     int64           `json:"id"`
	State  string          `json:"state"`
	Result json.RawMessage `json:"result"`
	Error  interface{}     `json:"error"`
}

func main() {
	_ = godotenv.Load()
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	apiKey := os.Getenv("TRUENAS_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("TRUENAS_API_KEY environment variable is required")
	}

	dnsName := os.Getenv("DNS_NAME")
	if dnsName == "" {
		return fmt.Errorf("DNS_NAME environment variable is required")
	}

	certName := getEnv("CERT_NAME", "Tailscale-Auto-Cert")
	serverURL := getEnv("TRUENAS_URL", "ws://192.168.1.5/api/websocket")

	fmt.Printf("Step 1: Generating/Updating certificate inside Tailscale container ix-tailscale-tailscale-1...\n")
	cmd := exec.Command("docker", "exec", "ix-tailscale-tailscale-1", "tailscale", "cert", dnsName)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to generate cert: %v (Output: %s)", err, string(out))
	}

	fmt.Println("Step 2 & 3: Extracting certificate and key...")
	certCmd := exec.Command("docker", "exec", "ix-tailscale-tailscale-1", "cat", dnsName+".crt")
	cert, err := certCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to extract cert: %v", err)
	}

	keyCmd := exec.Command("docker", "exec", "ix-tailscale-tailscale-1", "cat", dnsName+".key")
	key, err := keyCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to extract key: %v", err)
	}

	if err := verifyLocalCert(string(cert)); err != nil {
		return fmt.Errorf("local certificate validation failed: %v", err)
	}

	c, err := truenas_api.NewClient(serverURL, false)
	if err != nil {
		return fmt.Errorf("failed to create TN client: %v", err)
	}
	defer c.Close()

	return processCertificate(c, apiKey, certName, string(cert), string(key))
}

func processCertificate(c TNClient, apiKey, certName, cert, key string) error {
	if err := c.Login("", "", apiKey); err != nil {
		return fmt.Errorf("failed to login: %v", err)
	}

	fmt.Println("Step 4: Managing Certificate in TrueNAS...")
	raw, err := callAPI(c, "certificate.query", 60, []interface{}{
		[]interface{}{
			[]interface{}{"name", "=", certName},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to query certificates: %v", err)
	}

	var existingCerts []Certificate
	if err := json.Unmarshal(raw, &existingCerts); err != nil {
		return fmt.Errorf("failed to unmarshal existing certificates: %v", err)
	}

	var oldCertID int64
	if len(existingCerts) > 0 {
		oldCertID = existingCerts[0].ID
		newName := fmt.Sprintf("%s-old-%d", certName, time.Now().Unix())
		fmt.Printf("Renaming existing certificate (ID: %d) to %s to avoid conflict...\n", oldCertID, newName)

		updatePayload := CertificateUpdatePayload{
			Name: newName,
		}
		rawJob, err := callAPI(c, "certificate.update", 60, []interface{}{oldCertID, updatePayload})
		if err != nil {
			return fmt.Errorf("failed to trigger certificate rename: %v", err)
		}
		var jobID int64
		if err := json.Unmarshal(rawJob, &jobID); err != nil {
			return fmt.Errorf("failed to parse job ID from rename: %v", err)
		}
		if _, err := waitForJob(c, jobID); err != nil {
			return fmt.Errorf("certificate rename job failed: %v", err)
		}
	}

	fmt.Println("Creating new certificate...")
	createPayload := CertificateCreatePayload{
		Name:        certName,
		CreateType:  "CERTIFICATE_CREATE_IMPORTED",
		Certificate: cert,
		PrivateKey:  key,
		Passphrase:  nil,
	}

	rawJob, err := callAPI(c, "certificate.create", 60, []interface{}{createPayload})
	if err != nil {
		return fmt.Errorf("failed to trigger certificate creation: %v", err)
	}
	var jobID int64
	if err := json.Unmarshal(rawJob, &jobID); err != nil {
		return fmt.Errorf("failed to parse job ID from creation: %v", err)
	}

	result, err := waitForJob(c, jobID)
	if err != nil {
		return fmt.Errorf("certificate creation job failed: %v", err)
	}

	var certID int64
	if err := json.Unmarshal(result, &certID); err != nil {
		return fmt.Errorf("unexpected result type for certificate ID: %v (Value: %s)", err, string(result))
	}

	fmt.Printf("Step 5: Applying certificate ID %d to Web UI...\n", certID)
	_, err = callAPI(c, "system.general.update", 60, []interface{}{
		SystemGeneralUpdatePayload{UICertificate: certID},
	})
	if err != nil {
		return fmt.Errorf("failed to update system UI certificate: %v", err)
	}

	fmt.Println("Step 6: Restarting Web UI service...")
	_, err = callAPI(c, "system.general.ui_restart", 60, []interface{}{})
	if err != nil {
		fmt.Printf("Note: UI restart triggered (Connection might close): %v\n", err)
	}

	fmt.Println("Step 7: Verifying deployment...")
	if err := verifyDeployment(c, certID, cert); err != nil {
		return fmt.Errorf("deployment verification failed: %v", err)
	}

	if oldCertID != 0 {
		fmt.Printf("Step 8: Deleting old certificate (ID: %d)...\n", oldCertID)
		rawDelJob, err := callAPI(c, "certificate.delete", 60, []interface{}{oldCertID})
		if err != nil {
			fmt.Printf("Warning: failed to trigger old certificate deletion: %v\n", err)
		} else {
			var delJobID int64
			if err := json.Unmarshal(rawDelJob, &delJobID); err == nil {
				if _, err := waitForJob(c, delJobID); err != nil {
					fmt.Printf("Warning: old certificate deletion job failed: %v\n", err)
				}
			}
		}
	}

	fmt.Println("--- SUCCESS ---")
	return nil
}

func waitForJob(c TNClient, jobID int64) (json.RawMessage, error) {
	fmt.Printf("Job %d started, waiting for completion...\n", jobID)
	for {
		raw, err := callAPI(c, "core.get_jobs", 60, []interface{}{
			[]interface{}{
				[]interface{}{"id", "=", jobID},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to poll job status: %v", err)
		}

		var jobs []JobInfo
		if err := json.Unmarshal(raw, &jobs); err != nil {
			return nil, fmt.Errorf("failed to unmarshal job status: %v", err)
		}

		if len(jobs) == 0 {
			return nil, fmt.Errorf("job %d not found", jobID)
		}

		job := jobs[0]
		if job.State == "SUCCESS" {
			return job.Result, nil
		}
		if job.State == "FAILED" || job.State == "ABORTED" {
			return nil, fmt.Errorf("job %d finished with state: %s (Error: %v)", jobID, job.State, job.Error)
		}

		time.Sleep(1 * time.Second)
	}
}

func verifyDeployment(c TNClient, certID int64, expectedCert string) error {
	// 1. Verify certificate content
	raw, err := callAPI(c, "certificate.query", 60, []interface{}{
		[]interface{}{
			[]interface{}{"id", "=", certID},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to query certificate for verification: %v", err)
	}

	var certs []Certificate
	if err := json.Unmarshal(raw, &certs); err != nil {
		return fmt.Errorf("failed to unmarshal certificate: %v", err)
	}

	if len(certs) == 0 {
		return fmt.Errorf("certificate with ID %d not found after update", certID)
	}

	actualCert := strings.TrimSpace(certs[0].Certificate)
	expectedCertClean := strings.TrimSpace(expectedCert)

	if actualCert != expectedCertClean {
		// If direct comparison fails, compare parsed certificates to ignore formatting differences
		actualParsed, err := parsePEMCert(actualCert)
		if err != nil {
			return fmt.Errorf("failed to parse actual cert from TN: %v", err)
		}
		expectedParsed, err := parsePEMCert(expectedCertClean)
		if err != nil {
			return fmt.Errorf("failed to parse expected cert: %v", err)
		}

		if actualParsed.SerialNumber.Cmp(expectedParsed.SerialNumber) != 0 {
			return fmt.Errorf("certificate mismatch: serial numbers do not match (Expected: %s, Actual: %s)",
				expectedParsed.SerialNumber.String(), actualParsed.SerialNumber.String())
		}
		fmt.Println("Certificate content matches (via serial number verification).")
	} else {
		fmt.Println("Certificate content matches exactly.")
	}

	// 2. Verify system configuration
	rawConfig, err := callAPI(c, "system.general.config", 60, nil)
	if err != nil {
		return fmt.Errorf("failed to query system configuration: %v", err)
	}

	var config SystemGeneralConfig
	if err := json.Unmarshal(rawConfig, &config); err != nil {
		return fmt.Errorf("failed to unmarshal system configuration: %v", err)
	}

	if config.UICertificate != certID {
		return fmt.Errorf("system UI is NOT using the new certificate ID (Expected: %d, Actual: %d)",
			certID, config.UICertificate)
	}
	fmt.Printf("System UI is correctly using certificate ID %d.\n", certID)

	return nil
}

func parsePEMCert(pemData string) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}
	return x509.ParseCertificate(block.Bytes)
}

func callAPI(c TNClient, method string, timeout int64, params interface{}) (json.RawMessage, error) {
	raw, err := c.Call(method, timeout, params)
	if err != nil {
		return nil, err
	}

	if os.Getenv("DEBUG") == "true" {
		fmt.Printf("DEBUG: %s response: %s\n", method, string(raw))
	}

	var response RPCResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, fmt.Errorf("failed to parse API response envelope: %w", err)
	}

	if response.Error != nil {
		return nil, fmt.Errorf("API error: %s (Code: %d)", response.Error.Message, response.Error.Code)
	}

	return response.Result, nil
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func verifyLocalCert(certPEM string) error {
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil {
		return fmt.Errorf("failed to parse PEM block from certificate")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse x509 certificate: %v", err)
	}

	daysRemaining := time.Until(cert.NotAfter).Hours() / 24
	if daysRemaining < 30 {
		return fmt.Errorf("certificate expires in %.1f days, which is less than the required 30 days", daysRemaining)
	}

	fmt.Printf("Certificate is valid for %.1f more days.\n", daysRemaining)
	return nil
}
