package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type gateway struct {
	session *mcp.ClientSession
	mu      sync.Mutex
}

type planRequest struct {
	Message string `json:"message"`
}
type planResponse struct {
	ActionID         string         `json:"action_id"`
	Tool             string         `json:"tool"`
	Arguments        map[string]any `json:"arguments"`
	Summary          string         `json:"summary"`
	RequiresApproval bool           `json:"requires_approval"`
}
type executeRequest struct {
	ActionID  string         `json:"action_id"`
	Tool      string         `json:"tool"`
	Arguments map[string]any `json:"arguments"`
	Confirmed bool           `json:"confirmed"`
}

func main() {
	ctx := context.Background()
	command := os.Getenv("MCP_SERVER_COMMAND")
	if command == "" {
		command = "/usr/local/bin/payment-platform-mcp"
	}
	args := strings.Fields(os.Getenv("MCP_SERVER_ARGS"))
	client := mcp.NewClient(&mcp.Implementation{Name: "payment-platform-demo-agent", Version: "0.1.0"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: exec.CommandContext(ctx, command, args...)}, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer session.Close()
	g := &gateway{session: session}
	mux := http.NewServeMux()
	mux.HandleFunc("/demo/health", g.health)
	mux.HandleFunc("/demo/tools", g.tools)
	mux.HandleFunc("/demo/plan", g.plan)
	mux.HandleFunc("/demo/execute", g.execute)
	log.Println("demo gateway listening on :8081")
	log.Fatal(http.ListenAndServe(":8081", withJSON(mux)))
}

func withJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func writeJSON(w http.ResponseWriter, code int, value any) {
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(value)
}
func (g *gateway) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "connected"})
}
func (g *gateway) tools(w http.ResponseWriter, r *http.Request) {
	result, err := g.session.ListTools(r.Context(), nil)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"tools": result.Tools})
}

func (g *gateway) plan(w http.ResponseWriter, r *http.Request) {
	var input planRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || strings.TrimSpace(input.Message) == "" {
		writeJSON(w, 400, map[string]string{"error": "message is required"})
		return
	}
	message := strings.ToLower(input.Message)
	var plan planResponse
	plan.ActionID = newID()
	if isCreateRequest(message) {
		amount, currency := createAmount(input.Message)
		if amount <= 0 {
			writeJSON(w, 400, map[string]string{"error": "include a positive amount and currency, for example create a payment of 200 USD"})
			return
		}
		plan.Tool = "pay_payment"
		plan.Arguments = map[string]any{"amount": amount, "currency": currency, "capture_method": "manual", "reference": referenceFrom(input.Message), "idempotency_key": "demo-" + plan.ActionID}
		plan.Summary = fmt.Sprintf("Create and capture %s payment for %s", money(amount, currency), referenceFrom(input.Message))
		plan.RequiresApproval = true
	} else if strings.Contains(message, "refund") {
		id := firstUUID(message)
		amount := firstAmount(message)
		if id == "" || amount <= 0 {
			writeJSON(w, 400, map[string]string{"error": "include a payment ID and positive refund amount"})
			return
		}
		plan.Tool = "refund_payment"
		plan.Arguments = map[string]any{"payment_id": id, "amount": amount}
		plan.Summary = fmt.Sprintf("Refund %d minor units from payment %s", amount, id)
		plan.RequiresApproval = true
	} else if strings.Contains(message, "payment") || strings.Contains(message, "status") {
		id := firstUUID(message)
		if id == "" {
			writeJSON(w, 400, map[string]string{"error": "include a payment ID"})
			return
		}
		plan.Tool = "get_payment"
		plan.Arguments = map[string]any{"payment_id": id}
		plan.Summary = fmt.Sprintf("Retrieve the current state of payment %s", id)
	} else {
		writeJSON(w, 400, map[string]string{"error": "try asking to create, inspect, or refund a payment"})
		return
	}
	writeJSON(w, 200, plan)
}

func (g *gateway) execute(w http.ResponseWriter, r *http.Request) {
	var input executeRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.Tool == "" {
		writeJSON(w, 400, map[string]string{"error": "tool and arguments are required"})
		return
	}
	if (input.Tool == "refund_payment" || input.Tool == "pay_payment") && !input.Confirmed {
		writeJSON(w, 400, map[string]string{"error": "explicit approval is required"})
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	result, err := g.session.CallTool(r.Context(), &mcp.CallToolParams{Name: input.Tool, Arguments: input.Arguments})
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	if result.IsError {
		writeJSON(w, 502, map[string]string{"error": "MCP tool returned an error"})
		return
	}
	var value any
	if result.StructuredContent != nil {
		value = result.StructuredContent
	} else {
		value = result.Content
	}
	writeJSON(w, 200, map[string]any{"tool": input.Tool, "arguments": input.Arguments, "result": value})
}

var uuidPattern = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}`)
var createAmountPattern = regexp.MustCompile(`(?i)(\d+(?:\.\d{1,2})?)\s*(usd|eur|gbp|cad|aud)\b`)
var currencyPattern = regexp.MustCompile(`(?i)\b(usd|eur|gbp|cad|aud)\b`)

func firstUUID(value string) string { return uuidPattern.FindString(value) }
func firstAmount(value string) int64 {
	matches := regexp.MustCompile(`\b\d+\b`).FindAllString(value, -1)
	for _, match := range matches {
		n, _ := strconv.ParseInt(match, 10, 64)
		if n > 0 {
			return n
		}
	}
	return 0
}
func isCreateRequest(message string) bool {
	return (strings.Contains(message, "create") || strings.Contains(message, "charge") || strings.HasPrefix(strings.TrimSpace(message), "pay ")) && strings.Contains(message, "payment")
}
func createAmount(value string) (int64, string) {
	currency := "USD"
	if match := currencyPattern.FindStringSubmatch(value); len(match) > 1 {
		currency = strings.ToUpper(match[1])
	}
	match := createAmountPattern.FindStringSubmatch(value)
	if len(match) < 2 {
		return 0, currency
	}
	valueFloat, err := strconv.ParseFloat(match[1], 64)
	if err != nil || valueFloat <= 0 {
		return 0, currency
	}
	return int64(valueFloat*100 + 0.5), currency
}
func referenceFrom(value string) string {
	parts := strings.SplitN(strings.ToLower(value), " for ", 2)
	if len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
		return strings.TrimSpace(parts[1])
	}
	return "agent-demo-payment"
}
func money(amount int64, currency string) string {
	return fmt.Sprintf("%.2f %s", float64(amount)/100, currency)
}
func newID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("action-%d", time.Now().UnixNano())
	}
	return "action-" + hex.EncodeToString(b)
}
