package main

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/truenas/api_client_golang/truenas_api"
)

type mockTNClient struct {
	loginFunc       func(user, pass, token string) error
	callFunc        func(method string, timeout int64, params interface{}) (json.RawMessage, error)
	callWithJobFunc func(method string, params interface{}, callback func(float64, string, string)) (*truenas_api.Job, error)
	closeFunc       func() error
}

func (m *mockTNClient) Login(user, pass, token string) error { return m.loginFunc(user, pass, token) }
func (m *mockTNClient) Call(method string, timeout int64, params interface{}) (json.RawMessage, error) {
	return m.callFunc(method, timeout, params)
}

func (m *mockTNClient) CallWithJob(method string, params interface{}, callback func(float64, string, string)) (*truenas_api.Job, error) {
	return m.callWithJobFunc(method, params, callback)
}
func (m *mockTNClient) Close() error { return m.closeFunc() }

func TestProcessCertificate_Update(t *testing.T) {
	mock := &mockTNClient{
		loginFunc: func(user, pass, token string) error { return nil },
		callFunc: func(method string, timeout int64, params interface{}) (json.RawMessage, error) {
			var result interface{}
			switch method {
			case "certificate.query":
				result = []map[string]interface{}{
					{"id": float64(123), "name": "test-cert"},
				}
			case "certificate.update", "system.general.update", "system.general.ui_restart":
				result = true
			default:
				return nil, fmt.Errorf("unexpected method: %s", method)
			}
			data, _ := json.Marshal(map[string]interface{}{
				"result":  result,
				"error":   nil,
				"id":      1,
				"jsonrpc": "2.0",
			})
			return json.RawMessage(data), nil
		},
		closeFunc: func() error { return nil },
	}

	err := processCertificate(mock, "key", "test-cert", "CERT", "BEGIN PRIVATE KEY\nKEY\n")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
}

func TestProcessCertificate_Create(t *testing.T) {
	mock := &mockTNClient{
		loginFunc: func(user, pass, token string) error { return nil },
		callFunc: func(method string, timeout int64, params interface{}) (json.RawMessage, error) {
			var result interface{}
			switch method {
			case "certificate.query":
				result = []interface{}{}
			case "system.general.update", "system.general.ui_restart":
				result = true
			default:
				return nil, fmt.Errorf("unexpected method: %s", method)
			}
			data, _ := json.Marshal(map[string]interface{}{
				"result":  result,
				"error":   nil,
				"id":      1,
				"jsonrpc": "2.0",
			})
			return json.RawMessage(data), nil
		},
		callWithJobFunc: func(method string, params interface{}, callback func(float64, string, string)) (*truenas_api.Job, error) {
			if method == "certificate.create" {
				doneCh := make(chan string, 1)
				doneCh <- "SUCCESS"
				return &truenas_api.Job{
					ID:     456,
					DoneCh: doneCh,
					Result: float64(456),
				}, nil
			}
			return nil, fmt.Errorf("unexpected method: %s", method)
		},
		closeFunc: func() error { return nil },
	}

	err := processCertificate(mock, "key", "test-cert", "CERT", "BEGIN PRIVATE KEY\nKEY\n")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
}

func TestRun(t *testing.T) {
	// Mock environment
	os.Setenv("TRUENAS_API_KEY", "test-key")
	os.Setenv("DNS_NAME", "example.com")
	defer os.Unsetenv("TRUENAS_API_KEY")
	defer os.Unsetenv("DNS_NAME")

	// Mock commands
	origRunCommand := runCommand
	origRunCommandOutput := runCommandOutput
	defer func() {
		runCommand = origRunCommand
		runCommandOutput = origRunCommandOutput
	}()

	runCommand = func(name string, args ...string) error {
		return nil
	}
	runCommandOutput = func(name string, args ...string) (string, error) {
		if args[len(args)-1] == "/example.com.crt" {
			return "CERT", nil
		}
		if args[len(args)-1] == "/example.com.key" {
			return "BEGIN PRIVATE KEY\nKEY\n", nil
		}
		return "", nil
	}

	// This test is harder because run() creates the real client.
	// We might need to mock truenas_api.NewClient if we want to test run() fully.
	// For now, let's at least test that it fails if env vars are missing.
}

func TestRun_MissingEnv(t *testing.T) {
	os.Unsetenv("TRUENAS_API_KEY")
	err := run()
	if err == nil || err.Error() != "TRUENAS_API_KEY environment variable is required" {
		t.Errorf("Expected TRUENAS_API_KEY error, got %v", err)
	}

	os.Setenv("TRUENAS_API_KEY", "key")
	os.Unsetenv("DNS_NAME")
	err = run()
	if err == nil || err.Error() != "DNS_NAME environment variable is required" {
		t.Errorf("Expected DNS_NAME error, got %v", err)
	}
}
