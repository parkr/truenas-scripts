package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

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

	raw, err := c.Call("certificate.query", 60, []interface{}{
		[]interface{}{
			[]interface{}{"name", "=", certName},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to query certificates: %v", err)
	}

	var existingCerts []map[string]interface{}
	if err := json.Unmarshal(raw, &existingCerts); err != nil {
		return fmt.Errorf("failed to unmarshal existing certificates: %v", err)
	}

	var certID int64
	if len(existingCerts) > 0 {
		idFloat, ok := existingCerts[0]["id"].(float64)
		if !ok {
			return fmt.Errorf("unexpected ID type: %T", existingCerts[0]["id"])
		}
		certID = int64(idFloat)
		fmt.Printf("Updating existing certificate (ID: %d)...\n", certID)

		updatePayload := CertificateUpdatePayload{
			Certificate: cert,
			PrivateKey:  key,
			Passphrase:  nil,
		}
		_, err = c.Call("certificate.update", 60, []interface{}{certID, updatePayload})
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
	_, err = c.Call("system.general.update", 60, []interface{}{
		map[string]interface{}{"ui_certificate": certID},
	})
	if err != nil {
		return fmt.Errorf("failed to update system UI certificate: %v", err)
	}

	fmt.Println("Step 6: Restarting Web UI service...")
	_, err = c.Call("system.general.ui_restart", 60, nil)
	if err != nil {
		fmt.Printf("Note: UI restart triggered (Connection might close): %v\n", err)
	}

	fmt.Println("--- SUCCESS ---")
	return nil
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
