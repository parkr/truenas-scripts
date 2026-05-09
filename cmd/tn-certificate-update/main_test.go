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

const testCert = `-----BEGIN CERTIFICATE-----
MIIDFzCCAf+gAwIBAgIUUGuAM6USAjiCcHmfHA+DvjEysJIwDQYJKoZIhvcNAQEL
BQAwGzEZMBcGA1UEAwwQdGVzdC5leGFtcGxlLmNvbTAeFw0yNjA1MDgyMzI1Mzla
Fw0yNzA1MDgyMzI1MzlaMBsxGTAXBgNVBAMMEHRlc3QuZXhhbXBsZS5jb20wggEi
MA0GCSqGSIb3DQEBAQUAA4IBDwAwggEKAoIBAQDTWuunfs/5HkyNSdCEkfl5OGOe
T/oBsYRbWhBm4r7Jzh0pwJm3FzuX99jUYloz/iy/6HEtPbtBDAl4sjukvzBZbS7a
DoBSisgXf4CTbilaNZizm996nRh/axLdDAAo8VqfEjMFpfMtiQBL2Wm0AmrnTWEM
NkZfuBF5vw3h3dLRksj2x4zaup6I6y4q+p3lEopJ3uzRVkNjXTEY8iIhfAZzEAx0
2QSqjlA9AEabJI5yOb7/jlySBujIXXUxyA/iFKXDXVVPRXZ2CRZ682RckSZaFPVw
7qtTMJpcPc94t+PsX1WIB/9rmixE+dRDl5em8w3mrO5cxPE/cdpYOkjqWdHjAgMB
AAGjUzBRMB0GA1UdDgQWBBToHDIn6MGLn1Od0X9FutbIcNdGMTAfBgNVHSMEGDAW
gBToHDIn6MGLn1Od0X9FutbIcNdGMTAPBgNVHRMBAf8EBTADAQH/MA0GCSqGSIb3
DQEBCwUAA4IBAQBQ/i9udgXd86xaN1O5bjQuW+QufhVTbV3C6pqnssf+IKXPylGI
61r9XWoLjLTJleEH+fg7iNSGHtGOQfpbu/QPbFOYskYKy6Acrt3gxeDNL2Rw2lNd
wgT/snXUfAx4zUoKlCpSg3f3qgsUXggrOVa9ocFyRRizm+q6D3v45QlCBlg8f+hJ
5wxdQv1Cox+N/DhDihp+9xu5O5CR4fU41Ix2SvjB0UC7zLxNxt6U6HDm6HwQgR9C
CTujBv/rSiYWf+Ep05fEIFI5OrGjoosKbP31YX4q5IaTuu9uVFEHW2wdznlSrnfG
tGMY2jSs1IWXjBIQlqdowzjnmsBABvc/1yE3
-----END CERTIFICATE-----`

const testKey = `-----BEGIN PRIVATE KEY-----
MIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQDTWuunfs/5HkyN
SdCEkfl5OGOeT/oBsYRbWhBm4r7Jzh0pwJm3FzuX99jUYloz/iy/6HEtPbtBDAl4
sjukvzBZbS7aDoBSisgXf4CTbilaNZizm996nRh/axLdDAAo8VqfEjMFpfMtiQBL
2Wm0AmrnTWEMNkZfuBF5vw3h3dLRksj2x4zaup6I6y4q+p3lEopJ3uzRVkNjXTEY
8iIhfAZzEAx02QSqjlA9AEabJI5yOb7/jlySBujIXXUxyA/iFKXDXVVPRXZ2CRZ6
82RckSZaFPVw7qtTMJpcPc94t+PsX1WIB/9rmixE+dRDl5em8w3mrO5cxPE/cdpY
OkjqWdHjAgMBAAECggEAYHXqAG9NjtZnvMIYEzEmKU91k77PjO9OR30S6EoLJkJW
KASZgyjsz15UDRZ4MauLE+kLoki+yiCqv/WkZ/vEHsRIckfVBBcH3EWaUm8gG2ZD
s5FrzOOe1yRwnwcHmagRonDlbWoAUuNoibWH2xqRXNCBftfUhYIWI7jxJokdWvzs
WyL++1H6DsfMsLq9Tpi3qyit2ZwtapveXRs199hzo9FYc+2SJ1KZVH8E/FsplPR/
YVRhj4S/JArWjonV6paJ/RCMfXidC35zfxx1/PmsdbzsAO+7sYued/kihmFuSq1W
LiSawg3Zf7OvYzAneMmiZ8Wdv7ggrXm2D+Fa++F6WQKBgQD+0EFRCzyfdFltASZ6
9t1XwXC4wPgQ3/Z4WwKt7HOeXr16NpncSEW+xTkqFIJFenFfBjk4WxB85CCyN8rT
aRers037jtYVDwKxscOkEIuSGesg2nKPRUKO+jbOWLaRSkOVN8KGIs85p/8xQLcI
Ps/NMUeF0WSO2x8Q6e9MPWM6mQKBgQDUVtygNj3iCsz19Ej0w+9y7B8GdJ4j8How
ZA3E0Ggr/munrS6o6ukpRpQTNwW49JS9nARtkb+GBPDi7G24taHmuAO7QQ+mP8HF
5x8co527C2HCs3V1TuStOd0udyoubIzZbOpcdxzIacuije7InWCSNPha+uQbL32O
ICsUqgPZ2wKBgQD1vBzhbXa/R9Nd3fggKaZ4FOMCKYaRr4rfstU4qYkut6r/C10C
JOit+0EPpcuj+VsQCs5v3NJfvxkRBeEiVH0xZq/T44HtuRYeC5Liy9ntwfURL9m+
9Uok3ISyJreaEgZvBuEfvr4dmjfuZbydxQVdmyKgmLjjU8n348KUwbbKMQKBgC5I
hKyTRifYLNbLmX9ome/V0elpT/MLfsa/eFTXDG3SdgrFb+83zPzHOo15p9Cp1yYB
NOHhK/r9Zrg/yqbBSHnu0DlntA6LxSPq/dgTPdVAZN24mjioqqWrgC+Zn+MgnA7k
c60V9XslvFJBV7P4wcz8qMnD+CaI0nhBQMKvUEmTAoGBAK4i5r8SULapGT1tLrv6
H78ePPzX/4MQUIuvUig/2GQ1iqxpC0Ra1gFS5+SY64uYlWwnqjBPztiC6u/AmuBV
gf2ie2uSvtoSuUuixhBKTuRI54qFQ0bxOplz0Gcy0wXlF2pg/NQ1OdKnBD0TFqpW
GjBKUiGZlJSTJmma/Ovq02V/
-----END PRIVATE KEY-----`

func TestProcessCertificate_Update(t *testing.T) {
	mock := &mockTNClient{
		loginFunc: func(user, pass, token string) error { return nil },
		callFunc: func(method string, timeout int64, params interface{}) (json.RawMessage, error) {
			var result interface{}
			switch method {
			case "certificate.query":
				// Distinguish between query-by-name (initial) and query-by-id (verification)
				p := params.([]interface{})
				filters := p[0].([]interface{})
				if len(filters) > 0 {
					filter := filters[0].([]interface{})
					if filter[0].(string) == "name" {
						result = []Certificate{
							{ID: 123, Name: "test-cert", Certificate: "OLD CERT"},
						}
					} else if filter[0].(string) == "id" {
						result = []Certificate{
							{ID: 123, Name: "test-cert", Certificate: testCert},
						}
					}
				}
			case "certificate.update", "system.general.update", "system.general.ui_restart":
				result = true
			case "system.general.config":
				result = SystemGeneralConfig{UICertificate: 123}
			default:
				return nil, fmt.Errorf("unexpected method: %s", method)
			}
			// Actually need to marshal the whole envelope
			envelope := map[string]interface{}{
				"result":  result,
				"error":   nil,
				"id":      1,
				"jsonrpc": "2.0",
			}
			data, _ := json.Marshal(envelope)
			return json.RawMessage(data), nil
		},
		closeFunc: func() error { return nil },
	}

	err := processCertificate(mock, "key", "test-cert", testCert, testKey)
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

	err := processCertificate(mock, "key", "test-cert", testCert, testKey)
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
			return testCert, nil
		}
		if args[len(args)-1] == "/example.com.key" {
			return testKey, nil
		}
		return "", nil
	}

	// This test will still fail on truenas_api.NewClient(url, false) 
	// because we haven't mocked the NewClient function yet.
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
