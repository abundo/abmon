//
// Minimal client for the BECS EAPI (JSON-RPC 2.0), covering just what
// checkBecsDhcpScope needs: sessionLogin, rdrGet and sessionLogout.
//
// EAPI exposes the same methods/fields as the SOAP ExtAPI (see becs.wsdl,
// eapi.html) but wraps them in JSON-RPC 2.0, with the session id passed as
// a "_header": {"sessionid": ...} field inside params instead of a SOAP
// header.
//

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	abmon "github.com/abundo/abmon/internal"
)

type becsClient struct {
	url      string
	username string
	password string
	http     *http.Client

	sessionID string
	nextID    int
}

func newBecsClient(config *abmon.ConfigFile) (BecsClient, error) {
	if config.Becs.URL == "" {
		return nil, fmt.Errorf("becs.url is not configured in abmon.yaml")
	}
	if config.Becs.Username == "" {
		return nil, fmt.Errorf("becs.username is not configured in abmon.yaml")
	}
	return &becsClient{
		url:      config.Becs.URL,
		username: config.Becs.Username,
		password: config.Becs.Password,
		http:     &http.Client{Timeout: 30 * time.Second},
	}, nil
}

type jsonrpcRequest struct {
	Method  string `json:"method"`
	Params  any    `json:"params"`
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
}

type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type jsonrpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *jsonrpcError   `json:"error"`
}

// call issues one JSON-RPC request, logging in first if there is no session
// yet (sessionLogin itself is exempt, to avoid recursing into login).
func (c *becsClient) call(method string, params map[string]any) (json.RawMessage, error) {
	if method != "sessionLogin" && c.sessionID == "" {
		if err := c.login(); err != nil {
			return nil, err
		}
	}
	if c.sessionID != "" {
		if params == nil {
			params = map[string]any{}
		}
		params["_header"] = map[string]string{"sessionid": c.sessionID}
	}

	c.nextID++
	payload, err := json.Marshal(jsonrpcRequest{
		Method:  method,
		Params:  params,
		JSONRPC: "2.0",
		ID:      c.nextID,
	})
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Post(c.url, "application/json", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("becs %s: %w", method, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("becs %s: read: %w", method, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("becs %s: %s: %s", method, resp.Status, body)
	}

	var rpc jsonrpcResponse
	if err := json.Unmarshal(body, &rpc); err != nil {
		return nil, fmt.Errorf("becs %s: decode: %w", method, err)
	}
	if rpc.Error != nil {
		return nil, fmt.Errorf("becs %s: %s (code %d)", method, rpc.Error.Message, rpc.Error.Code)
	}
	return rpc.Result, nil
}

func (c *becsClient) login() error {
	result, err := c.call("sessionLogin", map[string]any{
		"username": c.username,
		"password": c.password,
	})
	if err != nil {
		return err
	}
	var out struct {
		ErrTxt    string `json:"errtxt"`
		SessionID string `json:"sessionid"`
	}
	if err := json.Unmarshal(result, &out); err != nil {
		return fmt.Errorf("becs sessionLogin: decode: %w", err)
	}
	if out.SessionID == "" {
		return fmt.Errorf("becs sessionLogin: %s", out.ErrTxt)
	}
	c.sessionID = out.SessionID
	return nil
}

// Logout best-effort logs out of the BECS session. It is called as deferred
// cleanup, so errors are dropped rather than surfaced.
func (c *becsClient) Logout() {
	if c.sessionID == "" {
		return
	}
	c.call("sessionLogout", nil)
	c.sessionID = ""
}

// rdrData is one BECS RDR (Resource Data Record) sample. A prefix-level
// entry carries its metrics nested in vdata (e.g. name "free"/"assigned"/
// "excluded" with a value in vgauge); it does not itself carry a vgauge.
type rdrData struct {
	Name   string    `json:"name"`
	VGauge int64     `json:"vgauge"`
	VData  []rdrData `json:"vdata"`
}

type rdrGetResult struct {
	Entries []struct {
		ErrTxt string    `json:"errtxt"`
		Data   []rdrData `json:"data"`
	} `json:"entries"`
}

func (c *becsClient) rdrGet(oid uint64, cname string) ([]rdrData, error) {
	result, err := c.call("rdrGet", map[string]any{
		"oid":   oid,
		"cname": map[string]string{"cname": cname},
	})
	if err != nil {
		return nil, err
	}
	var parsed rdrGetResult
	if err := json.Unmarshal(result, &parsed); err != nil {
		return nil, fmt.Errorf("becs rdrGet: decode: %w", err)
	}
	var out []rdrData
	for _, e := range parsed.Entries {
		if e.ErrTxt != "" {
			return nil, fmt.Errorf("becs rdrGet: %s", e.ErrTxt)
		}
		out = append(out, e.Data...)
	}
	return out, nil
}

// DHCPScopeUtilization sums the free/assigned/excluded address counts BECS
// reports for the "ipv4" RDR of the parameter-inet object at oid. A scope
// normally maps to a single prefix (one rdrData entry); if BECS returns
// more than one, their counts are added together.
func (c *becsClient) DHCPScopeUtilization(oid string) (*DHCPScopeUtilization, error) {
	oidNum, err := strconv.ParseUint(oid, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid oid %q: %w", oid, err)
	}
	prefixes, err := c.rdrGet(oidNum, "ipv4")
	if err != nil {
		return nil, err
	}
	var free, assigned, excluded int64
	for _, prefix := range prefixes {
		for _, v := range prefix.VData {
			switch v.Name {
			case "free":
				free += v.VGauge
			case "assigned":
				assigned += v.VGauge
			case "excluded":
				excluded += v.VGauge
			}
		}
	}
	return &DHCPScopeUtilization{
		Total:    int(free + assigned + excluded),
		Assigned: int(assigned),
		Free:     int(free),
	}, nil
}
