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

func main() {
	_ = godotenv.Load()

	apiKey := os.Getenv("TRUENAS_API_KEY")
	if apiKey == "" {
		log.Fatal("TRUENAS_API_KEY environment variable is required")
	}

	dnsName := getEnv("DNS_NAME", "dwyfron.whale-chickadee.ts.net")
	certName := getEnv("CERT_NAME", "Tailscale-Auto-Cert")
	containerName := getEnv("CONTAINER_NAME", "ix-tailscale-tailscale-1")
	apiHost := getEnv("TRUENAS_HOST", "localhost")

	fmt.Printf("Step 1: Generating/Updating certificate inside Tailscale container %s...\n", containerName)
	err := runCommand("sudo", "docker", "exec", containerName, "tailscale", "cert", "--min-validity", "720h", dnsName)
	if err != nil {
		log.Fatalf("Failed to generate cert: %v", err)
	}

	fmt.Println("Step 2 & 3: Extracting certificate and key...")
	cert, err := runCommandOutput("sudo", "docker", "exec", containerName, "cat", "/"+dnsName+".crt")
	if err != nil {
		log.Fatalf("Failed to read cert: %v", err)
	}
	key, err := runCommandOutput("sudo", "docker", "exec", containerName, "cat", "/"+dnsName+".key")
	if err != nil {
		log.Fatalf("Failed to read key: %v", err)
	}

	if !strings.Contains(key, "PRIVATE KEY") {
		log.Fatal("CRITICAL: Key extraction failed. Content check failed.")
	}

	fmt.Println("Step 4: Managing Certificate in TrueNAS...")
	// Use wss if not localhost, or ws if localhost
	protocol := "ws"
	if apiHost != "localhost" && apiHost != "127.0.0.1" {
		protocol = "wss"
	}
	url := fmt.Sprintf("%s://%s/api/current", protocol, apiHost)
	
	c, err := truenas_api.NewClient(url, false)
	if err != nil {
		log.Fatalf("Failed to connect to TrueNAS API: %v", err)
	}
	defer c.Close()

	if err := c.Login("", "", apiKey); err != nil {
		log.Fatalf("Failed to login: %v", err)
	}

	raw, err := c.Call("certificate.query", 60, []interface{}{
		[]interface{}{
			[]interface{}{"name", "=", certName},
		},
	})
	if err != nil {
		log.Fatalf("Failed to query certificates: %v", err)
	}

	var existingCerts []map[string]interface{}
	if err := json.Unmarshal(raw, &existingCerts); err != nil {
		log.Fatalf("Failed to unmarshal existing certificates: %v", err)
	}

	var certID int64
	if len(existingCerts) > 0 {
		idFloat, ok := existingCerts[0]["id"].(float64)
		if !ok {
			log.Fatalf("Unexpected ID type: %T", existingCerts[0]["id"])
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
			log.Fatalf("Failed to update certificate: %v", err)
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
			log.Fatalf("Failed to create certificate: %v", err)
		}
		
		fmt.Printf("Job %d started, waiting for completion...\n", job.ID)
		state := <-job.DoneCh
		if state != "SUCCESS" {
			log.Fatalf("Job failed with state: %s", state)
		}

		// After success, job.Result should contain the ID
		idFloat, ok := job.Result.(float64)
		if !ok {
			log.Fatalf("Unexpected result type: %T", job.Result)
		}
		certID = int64(idFloat)
	}

	fmt.Printf("Step 5: Applying certificate ID %d to Web UI...\n", certID)
	_, err = c.Call("system.general.update", 60, []interface{}{
		map[string]interface{}{"ui_certificate": certID},
	})
	if err != nil {
		log.Fatalf("Failed to update system UI certificate: %v", err)
	}

	fmt.Println("Step 6: Restarting Web UI service...")
	_, err = c.Call("system.general.ui_restart", 60, nil)
	if err != nil {
		// UI restart might close the connection, which is expected
		fmt.Printf("Note: UI restart triggered (Connection might close): %v\n", err)
	}

	fmt.Println("--- SUCCESS ---")
	fmt.Println("TrueNAS is now using the Tailscale HTTPS certificate.")
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runCommandOutput(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Stderr = os.Stderr
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
