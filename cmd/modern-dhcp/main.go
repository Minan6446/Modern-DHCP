package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"modern-dhcp/pkg/models"
)

const (
	defaultAPIBase        = "http://127.0.0.1:8080"
	defaultLimit          = 50
	limitMax              = 500
	stalenessThresholdDur = 30 * time.Minute
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		printRootHelp()
		return errors.New("missing command")
	}

	switch args[0] {
	case "help", "-h", "--help":
		printRootHelp()
		return nil
	case "diagnostics":
		return handleDiagnostics(args[1:])
	case "prefix-leases":
		return handlePrefixLeases(args[1:])
	case "leases":
		return handleLeases(args[1:])
	case "pools":
		return handlePools(args[1:])
	case "policy":
		return handlePolicy(args[1:])
	default:
		printRootHelp()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func printRootHelp() {
	fmt.Println(`modern-dhcp management CLI

Usage:
  modern-dhcp <command> [flags]

Commands:
	diagnostics collect Collect diagnostics snapshot (JSON or ZIP)
	leases list         List IPv4/IPv6 leases for a tenant (plus release/decline)
	prefix-leases list  List delegated IPv6 prefixes for a tenant (plus release/decline)
	pools find|resolve  Query or resolve pools by metadata
	policy selector     Generate policy selector JSON snippets

Flags:
  -h, --help           Show this help message`)
}

func handlePrefixLeases(args []string) error {
	if len(args) == 0 {
		printPrefixHelp()
		return errors.New("missing prefix-leases subcommand")
	}
	switch args[0] {
	case "list":
		return runPrefixLeaseList(args[1:])
	case "release":
		return runPrefixLeaseAction("release", args[1:])
	case "decline":
		return runPrefixLeaseAction("decline", args[1:])
	case "help", "-h", "--help":
		printPrefixHelp()
		return nil
	default:
		printPrefixHelp()
		return fmt.Errorf("unknown prefix-leases subcommand %q", args[0])
	}
}

func printPrefixHelp() {
	fmt.Println(`Usage:
  modern-dhcp prefix-leases <command> [flags]

Commands:
  list                 List delegated prefixes (supports filters/pagination)
  release              Release a prefix lease by ID
  decline              Mark a prefix lease as declined by ID

Common Flags:
	--tenant string       Tenant identifier (required)
	--url string          API base URL (default http://127.0.0.1:8080)
	--api-key string      API key to send via X-API-Key header
	--bearer string       Optional bearer token for Authorization header

List Flags:
	--state string        Optional state filter (ACTIVE, RELEASED, DECLINED, ...)
	--limit int           Page size (default 50, max 500)
	--offset int          Pagination offset (default 0)

Release/Decline Flags:
	--id string           Prefix lease ID (required)
	--reason string       Optional reason stored via audit trail`)
}

func runPrefixLeaseList(args []string) error {
	fs := flag.NewFlagSet("prefix-leases list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	tenantID := fs.String("tenant", "", "Tenant identifier (required)")
	state := fs.String("state", "", "State filter (ACTIVE, RELEASED, ...)")
	limit := fs.Int("limit", defaultLimit, "Page size (1-500)")
	offset := fs.Int("offset", 0, "Pagination offset")
	baseURL := fs.String("url", defaultAPIBase, "API base URL")
	apiKey := fs.String("api-key", "", "API key for X-API-Key header")
	bearer := fs.String("bearer", "", "Bearer token for Authorization header")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if strings.TrimSpace(*tenantID) == "" {
		return errors.New("tenant is required")
	}

	if *limit <= 0 {
		*limit = defaultLimit
	}
	if *limit > limitMax {
		*limit = limitMax
	}
	if *offset < 0 {
		*offset = 0
	}

	endpoint, err := buildPrefixLeaseURL(*baseURL, *tenantID, *state, *limit, *offset)
	if err != nil {
		return err
	}

	var leases []models.PrefixLease
	headers, err := doJSONRequest(http.MethodGet, endpoint, nil, *apiKey, *bearer, &leases)
	if err != nil {
		return err
	}
	noteWarning(headers)

	renderPrefixLeases(leases)
	return nil
}

func buildPrefixLeaseURL(base, tenant, state string, limit, offset int) (string, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(base), "/")
	if trimmed == "" {
		trimmed = defaultAPIBase
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("invalid base URL: %w", err)
	}
	tenantPath := url.PathEscape(tenant)
	u.Path = strings.TrimRight(u.Path, "/") + "/api/v1/tenants/" + tenantPath + "/prefix-leases"
	query := url.Values{}
	query.Set("limit", strconv.Itoa(limit))
	query.Set("offset", strconv.Itoa(offset))
	if state = strings.TrimSpace(state); state != "" {
		query.Set("state", state)
	}
	u.RawQuery = query.Encode()
	return u.String(), nil
}

func renderPrefixLeases(leases []models.PrefixLease) {
	if len(leases) == 0 {
		fmt.Println("No prefix leases found.")
		return
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "PREFIX\tLEN\tIAPD\tSTATE\tSTATUS\tEXPIRES_AT\tPOOL\tCLIENT")
	for _, lease := range leases {
		status := classifyLease(lease)
		fmt.Fprintf(tw, "%s\t/%d\t%d\t%s\t%s\t%s\t%s\t%s\n",
			lease.Prefix,
			lease.PrefixLen,
			lease.IAPDID,
			lease.State,
			status,
			formatTimestamp(lease.ExpiresAt),
			lease.PoolID,
			lease.ClientID,
		)
	}
	tw.Flush()
	fmt.Printf("\nTotal: %d\n", len(leases))
}

func classifyLease(lease models.PrefixLease) string {
	state := strings.ToUpper(lease.State)
	if state != "ACTIVE" {
		return strings.ToLower(state)
	}
	if time.Until(lease.ExpiresAt) <= stalenessThresholdDur {
		return "expiring"
	}
	return "ok"
}

func handlePools(args []string) error {
	if len(args) == 0 {
		printPoolsHelp()
		return errors.New("missing pools subcommand")
	}
	switch args[0] {
	case "find":
		return runPoolFind(args[1:])
	case "resolve":
		return runPoolResolve(args[1:])
	case "help", "-h", "--help":
		printPoolsHelp()
		return nil
	default:
		printPoolsHelp()
		return fmt.Errorf("unknown pools subcommand %q", args[0])
	}
}

func printPoolsHelp() {
	fmt.Println(`Usage:
  modern-dhcp pools <command> [flags]

Commands:
  find                 List pools filtered by metadata (scope/VLAN/interface/SSID/location)
  resolve              Resolve the most specific pool for relay metadata

Common Flags:
	--tenant string       Tenant identifier (required)
	--url string          API base URL (default http://127.0.0.1:8080)
	--api-key string      API key for X-API-Key header
	--bearer string       Bearer token for Authorization header

Find Flags:
	--scope string        Optional scope filter (GLOBAL/SUBNET/VLAN/PORT)
	--parent string       Optional parent pool ID
	--vlan int            VLAN identifier
	--interface string    Interface ID (Gi1/0/1, xe-0/0/1, etc.)
	--ssid string         Wireless SSID metadata
	--location string     Location metadata
	--limit int           Page size (default 50, max 500)

Resolve Flags:
	--vlan int            VLAN identifier
	--interface string    Interface ID (takes precedence)
	--ssid string         Wireless SSID metadata
	--location string     Location metadata`)
}

func runPoolFind(args []string) error {
	fs := flag.NewFlagSet("pools find", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	tenantID := fs.String("tenant", "", "Tenant identifier (required)")
	scope := fs.String("scope", "", "Scope filter")
	parent := fs.String("parent", "", "Parent pool ID")
	vlan := fs.Int("vlan", 0, "VLAN identifier")
	iface := fs.String("interface", "", "Interface identifier")
	ssid := fs.String("ssid", "", "SSID metadata")
	location := fs.String("location", "", "Location metadata")
	limit := fs.Int("limit", defaultLimit, "Page size (1-500)")
	baseURL := fs.String("url", defaultAPIBase, "API base URL")
	apiKey := fs.String("api-key", "", "API key for X-API-Key header")
	bearer := fs.String("bearer", "", "Bearer token")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*tenantID) == "" {
		return errors.New("tenant is required")
	}
	if *limit <= 0 {
		*limit = defaultLimit
	}
	if *limit > limitMax {
		*limit = limitMax
	}
	req := poolFindRequest{
		Scope:    strings.TrimSpace(*scope),
		Limit:    *limit,
		ParentID: trimStringPtr(*parent),
		VLANID:   optionalVLAN(*vlan),
	}
	req.InterfaceID = trimStringPtr(*iface)
	req.SSID = trimStringPtr(*ssid)
	req.Location = trimStringPtr(*location)

	endpoint, err := buildTenantEndpoint(*baseURL, *tenantID, "/pools/find")
	if err != nil {
		return err
	}
	var pools []models.AddressPool
	if _, err := doJSONRequest(http.MethodPost, endpoint, req, *apiKey, *bearer, &pools); err != nil {
		return err
	}
	renderPools(pools)
	return nil
}

func runPoolResolve(args []string) error {
	fs := flag.NewFlagSet("pools resolve", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	tenantID := fs.String("tenant", "", "Tenant identifier (required)")
	vlan := fs.Int("vlan", 0, "VLAN identifier")
	iface := fs.String("interface", "", "Interface identifier")
	ssid := fs.String("ssid", "", "SSID metadata")
	location := fs.String("location", "", "Location metadata")
	baseURL := fs.String("url", defaultAPIBase, "API base URL")
	apiKey := fs.String("api-key", "", "API key for X-API-Key header")
	bearer := fs.String("bearer", "", "Bearer token")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*tenantID) == "" {
		return errors.New("tenant is required")
	}
	req := poolResolveRequest{
		InterfaceID: strings.TrimSpace(*iface),
		SSID:        strings.TrimSpace(*ssid),
		Location:    strings.TrimSpace(*location),
	}
	if vlanPtr := optionalVLAN(*vlan); vlanPtr != nil {
		req.VLANID = *vlanPtr
	}
	if req.isEmpty() {
		return errors.New("at least one selector flag (interface, ssid, location, vlan) is required")
	}
	endpoint, err := buildTenantEndpoint(*baseURL, *tenantID, "/pools/resolve")
	if err != nil {
		return err
	}
	var pool models.AddressPool
	if _, err := doJSONRequest(http.MethodPost, endpoint, req, *apiKey, *bearer, &pool); err != nil {
		return err
	}
	renderPool(pool)
	return nil
}

type poolFindRequest struct {
	Scope       string  `json:"scope,omitempty"`
	ParentID    *string `json:"parentId,omitempty"`
	VLANID      *int    `json:"vlanId,omitempty"`
	InterfaceID *string `json:"interfaceId,omitempty"`
	SSID        *string `json:"ssid,omitempty"`
	Location    *string `json:"location,omitempty"`
	Limit       int     `json:"limit,omitempty"`
}

type poolResolveRequest struct {
	InterfaceID string `json:"interfaceId,omitempty"`
	SSID        string `json:"ssid,omitempty"`
	Location    string `json:"location,omitempty"`
	VLANID      int    `json:"vlanId,omitempty"`
}

func (p poolResolveRequest) isEmpty() bool {
	return strings.TrimSpace(p.InterfaceID) == "" && strings.TrimSpace(p.SSID) == "" && strings.TrimSpace(p.Location) == "" && p.VLANID == 0
}

func renderPools(pools []models.AddressPool) {
	if len(pools) == 0 {
		fmt.Println("No pools matched the filter.")
		return
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ID	SCOPE	NAME	VLAN	INTERFACE	SSID	LOCATION")
	for _, pool := range pools {
		fmt.Fprintf(tw, "%s	%s	%s	%s	%s	%s	%s\n",
			pool.ID,
			pool.Scope,
			pool.Name,
			valueOrDash(pool.VLANID),
			valueOrDash(pool.InterfaceID),
			valueOrDash(pool.SSID),
			valueOrDash(pool.Location),
		)
	}
	tw.Flush()
	fmt.Printf("\nTotal: %d\n", len(pools))
}

func renderPool(pool models.AddressPool) {
	fmt.Printf("Pool ID: %s\n", pool.ID)
	fmt.Printf("Name: %s\n", pool.Name)
	fmt.Printf("Scope: %s\n", pool.Scope)
	fmt.Printf("VLAN: %s\n", valueOrDash(pool.VLANID))
	fmt.Printf("Interface: %s\n", valueOrDash(pool.InterfaceID))
	fmt.Printf("SSID: %s\n", valueOrDash(pool.SSID))
	fmt.Printf("Location: %s\n", valueOrDash(pool.Location))
}

func handleLeases(args []string) error {
	if len(args) == 0 {
		printLeaseHelp()
		return errors.New("missing leases subcommand")
	}
	switch args[0] {
	case "list":
		return runLeaseList(args[1:])
	case "release":
		return runLeaseAction("release", args[1:])
	case "decline":
		return runLeaseAction("decline", args[1:])
	case "cooldown-clear":
		return runLeaseCooldownClear(args[1:])
	case "help", "-h", "--help":
		printLeaseHelp()
		return nil
	default:
		printLeaseHelp()
		return fmt.Errorf("unknown leases subcommand %q", args[0])
	}
}

func printLeaseHelp() {
	fmt.Println(`Usage:
  modern-dhcp leases <command> [flags]

Commands:
	list                 List leases (supports filters/pagination)
	release              Release a lease by ID
	decline              Mark a lease as declined by ID
	cooldown-clear       Clear cooldown/quarantine state so the IP can return to the pool

Common Flags:
	--tenant string       Tenant identifier (required)
	--url string          API base URL (default http://127.0.0.1:8080)
	--api-key string      API key to send via X-API-Key header
	--bearer string       Optional bearer token for Authorization header

List Flags:
	--state string        Optional lease state filter (ACTIVE, EXPIRED, DECLINED, ...)
	--limit int           Page size (default 50, max 500)
	--offset int          Pagination offset (default 0)

Release/Decline Flags:
	--lease string        Lease ID (required)
	--reason string       Optional reason stored via audit trail

Cooldown-Clear Flags:
	--lease string        Lease ID (required)`)
}

func runLeaseCooldownClear(args []string) error {
	fs := flag.NewFlagSet("leases cooldown-clear", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	tenantID := fs.String("tenant", "", "Tenant identifier (required)")
	leaseID := fs.String("lease", "", "Lease ID (required)")
	baseURL := fs.String("url", defaultAPIBase, "API base URL")
	apiKey := fs.String("api-key", "", "API key for X-API-Key header")
	bearer := fs.String("bearer", "", "Bearer token for Authorization header")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*tenantID) == "" {
		return errors.New("tenant is required")
	}
	if strings.TrimSpace(*leaseID) == "" {
		return errors.New("lease is required")
	}
	endpoint, err := buildCooldownClearURL(*baseURL, *tenantID, *leaseID)
	if err != nil {
		return err
	}
	var lease models.Lease
	headers, err := doJSONRequest(http.MethodPost, endpoint, nil, *apiKey, *bearer, &lease)
	if err != nil {
		return err
	}
	noteWarning(headers)
	printLeaseActionResult("cooldown-clear", lease)
	return nil
}

func runLeaseList(args []string) error {
	fs := flag.NewFlagSet("leases list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	tenantID := fs.String("tenant", "", "Tenant identifier (required)")
	state := fs.String("state", "", "State filter (ACTIVE, EXPIRED, ...)")
	limit := fs.Int("limit", defaultLimit, "Page size (1-500)")
	offset := fs.Int("offset", 0, "Pagination offset")
	baseURL := fs.String("url", defaultAPIBase, "API base URL")
	apiKey := fs.String("api-key", "", "API key for X-API-Key header")
	bearer := fs.String("bearer", "", "Bearer token for Authorization header")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if strings.TrimSpace(*tenantID) == "" {
		return errors.New("tenant is required")
	}

	if *limit <= 0 {
		*limit = defaultLimit
	}
	if *limit > limitMax {
		*limit = limitMax
	}
	if *offset < 0 {
		*offset = 0
	}

	endpoint, err := buildLeaseURL(*baseURL, *tenantID, *state, *limit, *offset)
	if err != nil {
		return err
	}

	var leases []models.Lease
	headers, err := doJSONRequest(http.MethodGet, endpoint, nil, *apiKey, *bearer, &leases)
	if err != nil {
		return err
	}
	noteWarning(headers)

	renderLeases(leases)
	return nil
}

func buildLeaseURL(base, tenant, state string, limit, offset int) (string, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(base), "/")
	if trimmed == "" {
		trimmed = defaultAPIBase
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("invalid base URL: %w", err)
	}
	tenantPath := url.PathEscape(tenant)
	u.Path = strings.TrimRight(u.Path, "/") + "/api/v1/tenants/" + tenantPath + "/leases"
	query := url.Values{}
	query.Set("limit", strconv.Itoa(limit))
	query.Set("offset", strconv.Itoa(offset))
	if state = strings.TrimSpace(state); state != "" {
		query.Set("state", state)
	}
	u.RawQuery = query.Encode()
	return u.String(), nil
}

func renderLeases(leases []models.Lease) {
	if len(leases) == 0 {
		fmt.Println("No leases found.")
		return
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "IP\tPOOL\tHWADDR\tCLIENT\tSTATE\tSTATUS\tEXPIRES_AT")
	for _, lease := range leases {
		status := classifyLeaseRecord(lease)
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			lease.IPAddress,
			lease.PoolID,
			lease.HardwareAddr,
			lease.ClientID,
			lease.State,
			status,
			formatTimestamp(lease.ExpiresAt),
		)
	}
	tw.Flush()
	fmt.Printf("\nTotal: %d\n", len(leases))
}

func classifyLeaseRecord(lease models.Lease) string {
	state := strings.ToUpper(lease.State)
	if state != "ACTIVE" {
		return strings.ToLower(state)
	}
	if time.Until(lease.ExpiresAt) <= stalenessThresholdDur {
		return "expiring"
	}
	return "ok"
}

func runLeaseAction(action string, args []string) error {
	label := fmt.Sprintf("leases %s", action)
	fs := flag.NewFlagSet(label, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	tenantID := fs.String("tenant", "", "Tenant identifier (required)")
	leaseID := fs.String("lease", "", "Lease ID (required)")
	reason := fs.String("reason", "", "Reason recorded for auditing")
	baseURL := fs.String("url", defaultAPIBase, "API base URL")
	apiKey := fs.String("api-key", "", "API key for X-API-Key header")
	bearer := fs.String("bearer", "", "Bearer token for Authorization header")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*tenantID) == "" {
		return errors.New("tenant is required")
	}
	if strings.TrimSpace(*leaseID) == "" {
		return errors.New("lease is required")
	}
	endpoint, err := buildActionURL(*baseURL, *tenantID, "leases", *leaseID, action)
	if err != nil {
		return err
	}
	body := map[string]string{}
	if trimmed := strings.TrimSpace(*reason); trimmed != "" {
		body["reason"] = trimmed
	} else {
		body = nil
	}
	var lease models.Lease
	headers, err := doJSONRequest(http.MethodPost, endpoint, body, *apiKey, *bearer, &lease)
	if err != nil {
		return err
	}
	noteWarning(headers)
	printLeaseActionResult(action, lease)
	return nil
}

func runPrefixLeaseAction(action string, args []string) error {
	label := fmt.Sprintf("prefix-leases %s", action)
	fs := flag.NewFlagSet(label, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	tenantID := fs.String("tenant", "", "Tenant identifier (required)")
	leaseID := fs.String("id", "", "Prefix lease ID (required)")
	reason := fs.String("reason", "", "Reason recorded for auditing")
	baseURL := fs.String("url", defaultAPIBase, "API base URL")
	apiKey := fs.String("api-key", "", "API key for X-API-Key header")
	bearer := fs.String("bearer", "", "Bearer token for Authorization header")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*tenantID) == "" {
		return errors.New("tenant is required")
	}
	if strings.TrimSpace(*leaseID) == "" {
		return errors.New("prefix lease id is required")
	}
	endpoint, err := buildActionURL(*baseURL, *tenantID, "prefix-leases", *leaseID, action)
	if err != nil {
		return err
	}
	body := map[string]string{}
	if trimmed := strings.TrimSpace(*reason); trimmed != "" {
		body["reason"] = trimmed
	} else {
		body = nil
	}
	var lease models.PrefixLease
	headers, err := doJSONRequest(http.MethodPost, endpoint, body, *apiKey, *bearer, &lease)
	if err != nil {
		return err
	}
	noteWarning(headers)
	printPrefixActionResult(action, lease)
	return nil
}

func handlePolicy(args []string) error {
	if len(args) == 0 {
		printPolicyHelp()
		return errors.New("missing policy subcommand")
	}
	switch args[0] {
	case "selector":
		return handlePolicySelector(args[1:])
	case "help", "-h", "--help":
		printPolicyHelp()
		return nil
	default:
		printPolicyHelp()
		return fmt.Errorf("unknown policy subcommand %q", args[0])
	}
}

func handleDiagnostics(args []string) error {
	if len(args) == 0 {
		printDiagnosticsHelp()
		return errors.New("missing diagnostics subcommand")
	}
	switch args[0] {
	case "collect":
		return runDiagnosticsCollect(args[1:])
	case "help", "-h", "--help":
		printDiagnosticsHelp()
		return nil
	default:
		printDiagnosticsHelp()
		return fmt.Errorf("unknown diagnostics subcommand %q", args[0])
	}
}

func printDiagnosticsHelp() {
	fmt.Println(`Usage:
  modern-dhcp diagnostics collect [flags]

Commands:
	collect              Download diagnostics snapshot (JSON by default, ZIP optional)

Flags:
	--tenant string       Tenant identifier used to enrich analytics (optional)
	--limit int           Analytics record limit (default 50, max 500)
	--format string       Output format: json or zip (default json)
	--output string       Write result to file (defaults to stdout for json, auto name for zip)
	--url string          API base URL (default http://127.0.0.1:8080)
	--api-key string      API key for X-API-Key header
	--bearer string       Bearer token for Authorization header`)
}

func runDiagnosticsCollect(args []string) error {
	fs := flag.NewFlagSet("diagnostics collect", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	tenantID := fs.String("tenant", "", "Tenant identifier used for analytics (optional)")
	limit := fs.Int("limit", defaultLimit, "Analytics record limit (1-500)")
	format := fs.String("format", "json", "Output format: json or zip")
	output := fs.String("output", "", "Path to write snapshot (stdout when empty for json)")
	baseURL := fs.String("url", defaultAPIBase, "API base URL")
	apiKey := fs.String("api-key", "", "API key for X-API-Key header")
	bearer := fs.String("bearer", "", "Bearer token for Authorization header")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if *limit <= 0 {
		*limit = defaultLimit
	}
	if *limit > limitMax {
		*limit = limitMax
	}
	formatValue := strings.ToLower(strings.TrimSpace(*format))
	if formatValue == "" {
		formatValue = "json"
	}
	endpoint, err := buildDiagnosticsURL(*baseURL, *tenantID, *limit, formatValue)
	if err != nil {
		return err
	}
	switch formatValue {
	case "json":
		var payload map[string]any
		if _, err := doJSONRequest(http.MethodGet, endpoint, nil, *apiKey, *bearer, &payload); err != nil {
			return err
		}
		pretty, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return err
		}
		target := strings.TrimSpace(*output)
		if target == "" || target == "-" {
			fmt.Println(string(pretty))
			return nil
		}
		if err := os.WriteFile(target, pretty, 0o644); err != nil {
			return err
		}
		fmt.Printf("Diagnostics snapshot written to %s\n", target)
		return nil
	case "zip":
		data, suggested, err := downloadDiagnosticsZip(endpoint, *apiKey, *bearer)
		if err != nil {
			return err
		}
		target := strings.TrimSpace(*output)
		if target == "" {
			if suggested != "" {
				target = suggested
			} else {
				target = defaultDiagnosticsZipName()
			}
		}
		if target == "-" {
			if _, err := os.Stdout.Write(data); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "\nwrote diagnostics archive (%d bytes) to stdout\n", len(data))
			return nil
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return err
		}
		fmt.Printf("Diagnostics archive written to %s (%d bytes)\n", target, len(data))
		return nil
	default:
		return fmt.Errorf("unsupported format %q (use json or zip)", formatValue)
	}
}

func printPolicyHelp() {
	fmt.Println(`Usage:
  modern-dhcp policy selector build [flags]

Commands:
  selector build        Emit policy action JSON containing poolSelector metadata

Selector Flags:
	--vlan int            VLAN identifier (integer > 0)
	--interface string    Interface identifier
	--ssid string         Wireless SSID metadata
	--location string     Location metadata
	--wrap                Wrap output inside {"poolSelector":{...}} (default true)
	--bare                Emit only the inner selector object`)
}

func handlePolicySelector(args []string) error {
	if len(args) == 0 {
		printPolicySelectorHelp()
		return errors.New("missing selector subcommand")
	}
	switch args[0] {
	case "build":
		return runPolicySelectorBuild(args[1:])
	case "help", "-h", "--help":
		printPolicySelectorHelp()
		return nil
	default:
		printPolicySelectorHelp()
		return fmt.Errorf("unknown selector subcommand %q", args[0])
	}
}

func printPolicySelectorHelp() {
	fmt.Println(`Usage:
  modern-dhcp policy selector build [flags]

Flags:
	--vlan int            VLAN identifier (integer > 0)
	--interface string    Interface identifier
	--ssid string         Wireless SSID metadata
	--location string     Location metadata
	--wrap                Wrap output inside {"poolSelector":{...}} (default true)
	--bare                Emit only the inner selector object (sets --wrap=false)`)
}

func runPolicySelectorBuild(args []string) error {
	fs := flag.NewFlagSet("policy selector build", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	vlan := fs.Int("vlan", 0, "VLAN identifier")
	iface := fs.String("interface", "", "Interface identifier")
	ssid := fs.String("ssid", "", "SSID metadata")
	location := fs.String("location", "", "Location metadata")
	wrap := fs.Bool("wrap", true, "Wrap selector inside {\"poolSelector\":{...}}")
	bare := fs.Bool("bare", false, "Emit only the inner selector object")
	if err := fs.Parse(args); err != nil {
		return err
	}
	selector := make(map[string]any)
	if vlanPtr := optionalVLAN(*vlan); vlanPtr != nil {
		selector["vlanId"] = *vlanPtr
	}
	if trimmed := strings.TrimSpace(*iface); trimmed != "" {
		selector["interfaceId"] = trimmed
	}
	if trimmed := strings.TrimSpace(*ssid); trimmed != "" {
		selector["ssid"] = trimmed
	}
	if trimmed := strings.TrimSpace(*location); trimmed != "" {
		selector["location"] = trimmed
	}
	if len(selector) == 0 {
		return errors.New("provide at least one selector flag")
	}
	if *bare {
		*wrap = false
	}
	var payload any = selector
	if *wrap {
		payload = map[string]any{"poolSelector": selector}
	}
	buf, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(buf))
	return nil
}

func doJSONRequest(method, endpoint string, payload any, apiKey, bearer string, out any) (http.Header, error) {
	var body io.Reader
	if payload != nil {
		buf := &bytes.Buffer{}
		if err := json.NewEncoder(buf).Encode(payload); err != nil {
			return nil, err
		}
		body = buf
	}
	req, err := http.NewRequest(method, endpoint, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if key := strings.TrimSpace(apiKey); key != "" {
		req.Header.Set("X-API-Key", key)
	}
	if token := strings.TrimSpace(bearer); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		return nil, fmt.Errorf("API request failed: %s - %s", resp.Status, strings.TrimSpace(string(data)))
	}
	if out == nil {
		return resp.Header, nil
	}
	return resp.Header, json.NewDecoder(resp.Body).Decode(out)
}

func noteWarning(headers http.Header) {
	if headers == nil {
		return
	}
	if warn := headers.Get("Warning"); warn != "" {
		fmt.Fprintf(os.Stderr, "warning: %s\n", warn)
	}
}

func buildActionURL(base, tenant, resource, resourceID, action string) (string, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(base), "/")
	if trimmed == "" {
		trimmed = defaultAPIBase
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("invalid base URL: %w", err)
	}
	tenantPath := url.PathEscape(strings.TrimSpace(tenant))
	resourcePath := strings.Trim(strings.TrimSpace(resource), "/")
	resourceIDPath := url.PathEscape(strings.TrimSpace(resourceID))
	u.Path = strings.TrimRight(u.Path, "/") + "/api/v1/tenants/" + tenantPath + "/" + resourcePath + "/" + resourceIDPath + "/" + strings.TrimSpace(action)
	return u.String(), nil
}

func buildCooldownClearURL(base, tenant, leaseID string) (string, error) {
	suffix := "/leases/" + url.PathEscape(strings.TrimSpace(leaseID)) + "/cooldown/clear"
	return buildTenantEndpoint(base, tenant, suffix)
}

func printLeaseActionResult(action string, lease models.Lease) {
	fmt.Printf("Lease %s %s (state=%s, ip=%s)\n", lease.ID, actionPastTense(action), lease.State, lease.IPAddress)
}

func printPrefixActionResult(action string, lease models.PrefixLease) {
	fmt.Printf("Prefix lease %s %s (state=%s, prefix=%s/%d)\n", lease.ID, actionPastTense(action), lease.State, lease.Prefix, lease.PrefixLen)
}

func actionPastTense(action string) string {
	switch strings.ToLower(action) {
	case "release":
		return "released"
	case "decline":
		return "declined"
	case "cooldown-clear":
		return "cooldown cleared"
	default:
		return action + "d"
	}
}

func formatTimestamp(ts time.Time) string {
	if ts.IsZero() {
		return ""
	}
	return ts.UTC().Format(time.RFC3339)
}

func buildTenantEndpoint(base, tenant, suffix string) (string, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(base), "/")
	if trimmed == "" {
		trimmed = defaultAPIBase
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("invalid base URL: %w", err)
	}
	tenantPath := url.PathEscape(strings.TrimSpace(tenant))
	sanitizedSuffix := suffix
	if !strings.HasPrefix(sanitizedSuffix, "/") {
		sanitizedSuffix = "/" + sanitizedSuffix
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/api/v1/tenants/" + tenantPath + sanitizedSuffix
	return u.String(), nil
}

func buildDiagnosticsURL(base, tenant string, limit int, format string) (string, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(base), "/")
	if trimmed == "" {
		trimmed = defaultAPIBase
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("invalid base URL: %w", err)
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/api/v1/diagnostics/snapshot"
	query := url.Values{}
	if limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	}
	if tenantID := strings.TrimSpace(tenant); tenantID != "" {
		query.Set("tenantId", tenantID)
	}
	if format == "zip" {
		query.Set("format", "zip")
	}
	u.RawQuery = query.Encode()
	return u.String(), nil
}

func defaultDiagnosticsZipName() string {
	return fmt.Sprintf("diagnostics_%s.zip", time.Now().UTC().Format("20060102T150405Z"))
}

func downloadDiagnosticsZip(endpoint, apiKey, bearer string) ([]byte, string, error) {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Accept", "application/zip")
	if key := strings.TrimSpace(apiKey); key != "" {
		req.Header.Set("X-API-Key", key)
	}
	if token := strings.TrimSpace(bearer); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		return nil, "", fmt.Errorf("API request failed: %s - %s", resp.Status, strings.TrimSpace(string(data)))
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	return data, parseAttachmentFilename(resp.Header.Get("Content-Disposition")), nil
}

func parseAttachmentFilename(disposition string) string {
	if disposition == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(disposition)
	if err != nil {
		return ""
	}
	if filename, ok := params["filename"]; ok {
		return filename
	}
	return ""
}

func trimStringPtr(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func optionalVLAN(v int) *int {
	if v <= 0 {
		return nil
	}
	return &v
}

func valueOrDash(value any) string {
	switch v := value.(type) {
	case *int:
		if v == nil || *v == 0 {
			return "-"
		}
		return strconv.Itoa(*v)
	case *string:
		if v == nil {
			return "-"
		}
		trimmed := strings.TrimSpace(*v)
		if trimmed == "" {
			return "-"
		}
		return trimmed
	default:
		return "-"
	}
}
