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

type CertificateUpdatePayload struct {
	Certificate string  `json:"certificate"`
	PrivateKey  string  `json:"privatekey"`
	Passphrase  *string `json:"passphrase"`
}

type CertificateCreatePayload struct {
	Name        string  `json:"name"`
	CreateType  string  `json:"create_type"`
	Certificate string  `json:"certificate"`
	PrivateKey  string  `json:"privatekey"`
	Passphrase  *string `json:"passphrase"`
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
}

type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("API error %d: %s", e.Code, e.Message)
}

var runCommand = func(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

var runCommandOutput = func(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Stderr = os.Stderr
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

type TNClient interface {
	Login(user, pass, token string) error
	Call(method string, timeout int64, params interface{}) (json.RawMessage, error)
	CallWithJob(method string, params interface{}, callback func(float64, string, string)) (*truenas_api.Job, error)
	Close() error
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
	containerName := getEnv("CONTAINER_NAME", "ix-tailscale-tailscale-1")
	apiHost := getEnv("TRUENAS_HOST", "localhost")

	fmt.Printf("Step 1: Generating/Updating certificate inside Tailscale container %s...\n", containerName)
	err := runCommand("sudo", "docker", "exec", containerName, "tailscale", "cert", "--min-validity", "720h", dnsName)
	if err != nil {
		return fmt.Errorf("failed to generate cert: %v", err)
	}

	fmt.Println("Step 2 & 3: Extracting certificate and key...")
	cert, err := runCommandOutput("sudo", "docker", "exec", containerName, "cat", "/"+dnsName+".crt")
	if err != nil {
		return fmt.Errorf("failed to read cert: %v", err)
	}
	key, err := runCommandOutput("sudo", "docker", "exec", containerName, "cat", "/"+dnsName+".key")
	if err != nil {
		return fmt.Errorf("failed to read key: %v", err)
	}

	if !strings.Contains(key, "PRIVATE KEY") {
		return fmt.Errorf("CRITICAL: Key extraction failed. Content check failed")
	}

	if err := verifyCertValidity(cert); err != nil {
		return fmt.Errorf("certificate validity check failed: %v", err)
	}

	fmt.Println("Step 4: Managing Certificate in TrueNAS...")
	protocol := "ws"
	if apiHost != "localhost" && apiHost != "127.0.0.1" {
		protocol = "wss"
	}
	url := fmt.Sprintf("%s://%s/api/current", protocol, apiHost)

	c, err := truenas_api.NewClient(url, false)
	if err != nil {
		return fmt.Errorf("failed to connect to TrueNAS API: %v", err)
	}
	defer c.Close()

	return processCertificate(c, apiKey, certName, cert, key)
}

func processCertificate(c TNClient, apiKey, certName, cert, key string) error {
	if err := c.Login("", "", apiKey); err != nil {
		return fmt.Errorf("failed to login: %v", err)
	}

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

	var certID int64
	if len(existingCerts) > 0 {
		certID = existingCerts[0].ID
		fmt.Printf("Updating existing certificate (ID: %d)...\n", certID)

		updatePayload := CertificateUpdatePayload{
			Certificate: cert,
			PrivateKey:  key,
			Passphrase:  nil,
		}
		_, err = callAPI(c, "certificate.update", 60, []interface{}{certID, updatePayload})
		if err != nil {
			return fmt.Errorf("failed to update certificate: %v", err)
		}
	} else {
		fmt.Println("Creating new certificate...")
		createPayload := CertificateCreatePayload{
			Name:        certName,
			CreateType:  "CERTIFICATE_CREATE_IMPORTED",
			Certificate: cert,
			PrivateKey:  key,
			Passphrase:  nil,
		}

		job, err := c.CallWithJob("certificate.create", []interface{}{createPayload}, nil)
		if err != nil {
			return fmt.Errorf("failed to create certificate: %v", err)
		}

		fmt.Printf("Job %d started, waiting for completion...\n", job.ID)
		state := <-job.DoneCh
		if state != "SUCCESS" {
			return fmt.Errorf("job failed with state: %s", state)
		}

		idFloat, ok := job.Result.(float64)
		if !ok {
			return fmt.Errorf("unexpected result type: %T", job.Result)
		}
		certID = int64(idFloat)
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

	fmt.Println("--- SUCCESS ---")
	return nil
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

	var response RPCResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, fmt.Errorf("failed to parse API response envelope: %w", err)
	}

	if response.Error != nil {
		return nil, response.Error
	}

	return response.Result, nil
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func verifyCertValidity(certPEM string) error {
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil || block.Type != "CERTIFICATE" {
		return fmt.Errorf("failed to decode certificate PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse certificate: %v", err)
	}

	daysRemaining := time.Until(cert.NotAfter).Hours() / 24
	if daysRemaining < 30 {
		return fmt.Errorf("certificate expires in %.1f days, which is less than the required 30 days", daysRemaining)
	}

	fmt.Printf("Certificate is valid for %.1f more days.\n", daysRemaining)
	return nil
}
