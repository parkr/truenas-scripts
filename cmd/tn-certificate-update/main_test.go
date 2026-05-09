package main

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/truenas/api_client_golang/truenas_api"
)

type mockTNClient struct {
	loginFunc           func(user, pass, token string) error
	callFunc            func(method string, timeout int64, params interface{}) (json.RawMessage, error)
	callWithJobFunc     func(method string, params interface{}, callback func(float64, string, string)) (*truenas_api.Job, error)
	subscribeToJobsFunc func() error
	closeFunc           func() error
}

func (m *mockTNClient) Login(user, pass, token string) error {
	return m.loginFunc(user, pass, token)
}

func (m *mockTNClient) Call(method string, timeout int64, params interface{}) (json.RawMessage, error) {
	return m.callFunc(method, timeout, params)
}

func (m *mockTNClient) CallWithJob(method string, params interface{}, callback func(float64, string, string)) (*truenas_api.Job, error) {
	if m.callWithJobFunc != nil {
		return m.callWithJobFunc(method, params, callback)
	}
	doneCh := make(chan string, 1)
	doneCh <- ""
	return &truenas_api.Job{
		ID:       1,
		DoneCh:   doneCh,
		Result:   456.0,
		Finished: true,
		State:    "SUCCESS",
	}, nil
}

func (m *mockTNClient) SubscribeToJobs() error {
	if m.subscribeToJobsFunc != nil {
		return m.subscribeToJobsFunc()
	}
	return nil
}

func (m *mockTNClient) Close() error {
	return m.closeFunc()
}

const testCert = `-----BEGIN CERTIFICATE-----
MIIDDTCCAfWgAwIBAgIUStV0GfFpW+f1u9O4G9P7Z7D4A4IwDQYJKoZIhvcNAQEL
BQAwFzEVMBMGA1UEAwwMdGVzdC1kb21haW4wIBcNMjYwNTA4MjIzMDU0WhgPMjEy
NjA0MTQyMjMwNTRaMBcxFTATBgNVBAMMDHRlc3QtZG9tYWluMIIBIjANBgkqhkiG
9w0BAQEFAAOCAQ8AMIIBCgKCAQEAyvU7k8gL+XGqjYFz9vHqL6Fv/k3r5F1x7w6L
...
-----END CERTIFICATE-----`

const testKey = `-----BEGIN PRIVATE KEY-----
MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQDK9TuTyAv9caqN
gXP28eovoa/+TevkXXHvDovm8p6u8p6u8p6u8p6u8p6u8p6u8p6u8p6u8p6u8p6u
...
-----END PRIVATE KEY-----`

func TestProcessCertificate_Update(t *testing.T) {
	var renamed, created, deleted bool
	mock := &mockTNClient{
		loginFunc: func(user, pass, token string) error { return nil },
		callFunc: func(method string, timeout int64, params interface{}) (json.RawMessage, error) {
			var result interface{}
			switch method {
			case "certificate.query":
				p := params.([]interface{})
				filters := p[0].([]interface{})
				if len(filters) > 0 {
					filter := filters[0].([]interface{})
					if filter[0].(string) == "name" {
						if !renamed {
							result = []Certificate{
								{ID: 123, Name: "test-cert", Certificate: "OLD CERT"},
							}
						} else {
							result = []Certificate{}
						}
					} else if filter[0].(string) == "id" {
						result = []Certificate{
							{ID: 456, Name: "test-cert", Certificate: testCert},
						}
					}
				}
			case "system.general.update", "system.general.ui_restart":
				result = true
			case "system.general.config":
				result = SystemGeneralConfig{UICertificate: 456}
			default:
				return nil, fmt.Errorf("unexpected method: %s", method)
			}
			envelope := map[string]interface{}{
				"result":  result,
				"error":   nil,
				"id":      1,
				"jsonrpc": "2.0",
			}
			data, _ := json.Marshal(envelope)
			return json.RawMessage(data), nil
		},
		callWithJobFunc: func(method string, params interface{}, callback func(float64, string, string)) (*truenas_api.Job, error) {
			doneCh := make(chan string, 1)
			doneCh <- ""
			var result interface{}
			switch method {
			case "certificate.update":
				renamed = true
				result = true
			case "certificate.create":
				created = true
				result = 456.0
			case "certificate.delete":
				deleted = true
				result = true
			}
			return &truenas_api.Job{
				ID:       1,
				DoneCh:   doneCh,
				Result:   result,
				Finished: true,
				State:    "SUCCESS",
			}, nil
		},
		closeFunc: func() error { return nil },
	}

	err := processCertificate(mock, "key", "test-cert", testCert, testKey)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if !renamed {
		t.Error("Expected certificate to be renamed")
	}
	if !created {
		t.Error("Expected new certificate to be created")
	}
	if !deleted {
		t.Error("Expected old certificate to be deleted")
	}
}

func TestProcessCertificate_Create(t *testing.T) {
	var created bool
	mock := &mockTNClient{
		loginFunc: func(user, pass, token string) error { return nil },
		callFunc: func(method string, timeout int64, params interface{}) (json.RawMessage, error) {
			var result interface{}
			switch method {
			case "certificate.query":
				if params != nil {
					p := params.([]interface{})
					filters := p[0].([]interface{})
					if len(filters) > 0 {
						filter := filters[0].([]interface{})
						if filter[0].(string) == "id" {
							result = []Certificate{
								{ID: 456, Name: "test-cert", Certificate: testCert},
							}
						} else {
							result = []interface{}{}
						}
					} else {
						result = []interface{}{}
					}
				} else {
					result = []interface{}{}
				}
			case "system.general.update", "system.general.ui_restart":
				result = true
			case "system.general.config":
				result = SystemGeneralConfig{UICertificate: 456}
			default:
				return nil, fmt.Errorf("unexpected method: %s", method)
			}
			envelope := map[string]interface{}{
				"result":  result,
				"error":   nil,
				"id":      1,
				"jsonrpc": "2.0",
			}
			data, _ := json.Marshal(envelope)
			return json.RawMessage(data), nil
		},
		callWithJobFunc: func(method string, params interface{}, callback func(float64, string, string)) (*truenas_api.Job, error) {
			doneCh := make(chan string, 1)
			doneCh <- ""
			var result interface{}
			switch method {
			case "certificate.create":
				created = true
				result = 456.0
			}
			return &truenas_api.Job{
				ID:       1,
				DoneCh:   doneCh,
				Result:   result,
				Finished: true,
				State:    "SUCCESS",
			}, nil
		},
		closeFunc: func() error { return nil },
	}

	err := processCertificate(mock, "key", "test-cert", testCert, testKey)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if !created {
		t.Error("Expected new certificate to be created")
	}
}

func TestRun_MissingEnv(t *testing.T) {
	os.Clearenv()
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
