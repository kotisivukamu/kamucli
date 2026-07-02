package status

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/kotisivukamu/kamucli/internal/command"
	"github.com/kotisivukamu/kamucli/internal/iostreams"
	"github.com/kotisivukamu/kamucli/internal/render"
)

func newMonitors() *cobra.Command {
	cmd := command.New("monitors", "Manage monitors", "", nil)
	cmd.Aliases = []string{"mon"}
	cmd.AddCommand(
		newMonitorsList(),
		newMonitorsAdd(),
		newMonitorsShow(),
		newMonitorsStats(),
		newMonitorsToggle("enable", true),
		newMonitorsToggle("disable", false),
		newMonitorsDelete(),
	)
	return cmd
}

func newMonitorsList() *cobra.Command {
	var asJSON bool
	cmd := command.New("list", "List monitors for a project", "", func(ctx context.Context, args []string) error {
		c, err := client()
		if err != nil {
			return err
		}
		monitors, err := c.ListMonitors(ctxOrTodo(ctx), args[0])
		if err != nil {
			return err
		}
		io := iostreams.FromContext(ctx)
		if asJSON {
			return render.JSON(io.Out, monitors)
		}
		if len(monitors) == 0 {
			fmt.Fprintln(io.Out, "No monitors.")
			return nil
		}
		rows := make([][]string, 0, len(monitors))
		for _, m := range monitors {
			state := "ON"
			if !m.Enabled {
				state = "OFF"
			}
			rows = append(rows, []string{m.ID, m.Name, m.Type, m.Target, fmt.Sprintf("%ds", m.IntervalSeconds), state})
		}
		return render.Table(io.Out, []string{"ID", "NAME", "TYPE", "TARGET", "INTERVAL", "STATE"}, rows)
	})
	cmd.Args = cobra.ExactArgs(1)
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output JSON")
	return cmd
}

func newMonitorsAdd() *cobra.Command {
	var (
		name, target, typ, method, dnsType, bodyContains string
		interval, expectedStatus, timeout, port, grace   int
	)
	cmd := command.New("add", "Add a monitor", "", func(ctx context.Context, args []string) error {
		if typ != "heartbeat" && target == "" {
			return fmt.Errorf("--target is required for %s monitors", typ)
		}
		input := map[string]any{"name": name, "type": typ}
		if target != "" {
			input["target"] = target
		}
		if method != "" {
			input["method"] = method
		}
		if interval > 0 {
			input["intervalSeconds"] = interval
		}
		if expectedStatus > 0 {
			input["expectedStatus"] = expectedStatus
		}
		if timeout > 0 {
			input["timeoutMs"] = timeout
		}
		if port > 0 {
			input["port"] = port
		}
		if dnsType != "" {
			input["dnsRecordType"] = dnsType
		}
		if bodyContains != "" {
			input["bodyContains"] = bodyContains
		}
		if grace > 0 {
			input["graceSeconds"] = grace
		}

		c, err := client()
		if err != nil {
			return err
		}
		data, err := c.CreateMonitor(ctxOrTodo(ctx), args[0], input)
		if err != nil {
			return err
		}
		var m struct {
			ID              string `json:"id"`
			Name            string `json:"name"`
			Type            string `json:"type"`
			Target          string `json:"target"`
			IntervalSeconds int    `json:"interval_seconds"`
			PingURL         string `json:"pingUrl"`
		}
		if err := json.Unmarshal(data, &m); err != nil {
			return err
		}
		io := iostreams.FromContext(ctx)
		fmt.Fprintf(io.Out, "Created monitor %s\n", m.Name)
		fmt.Fprintf(io.Out, "  id:     %s\n", m.ID)
		fmt.Fprintf(io.Out, "  type:   %s\n", m.Type)
		if m.Target != "" {
			fmt.Fprintf(io.Out, "  target: %s\n", m.Target)
		}
		if m.PingURL != "" {
			fmt.Fprintf(io.Out, "\nPing URL: %s\n", m.PingURL)
			fmt.Fprintf(io.Out, "Hit at least every %ds to keep the monitor green.\n", m.IntervalSeconds)
		}
		return nil
	})
	cmd.Args = cobra.ExactArgs(1)
	cmd.Flags().StringVar(&name, "name", "", "Monitor name")
	cmd.Flags().StringVar(&target, "target", "", "Target URL/hostname")
	cmd.Flags().StringVar(&typ, "type", "http", "Monitor type: http, tcp, dns, ping, heartbeat")
	cmd.Flags().StringVar(&method, "method", "", "HTTP method (defaults to GET)")
	cmd.Flags().IntVar(&interval, "interval", 0, "Check interval in seconds")
	cmd.Flags().IntVar(&expectedStatus, "expected-status", 0, "Expected HTTP status code")
	cmd.Flags().IntVar(&timeout, "timeout", 0, "Timeout in ms")
	cmd.Flags().IntVar(&port, "port", 0, "TCP port")
	cmd.Flags().StringVar(&dnsType, "dns-type", "", "DNS record type: A, AAAA, CNAME, MX, TXT")
	cmd.Flags().StringVar(&bodyContains, "body-contains", "", "Expected substring in response body")
	cmd.Flags().IntVar(&grace, "grace", 0, "Grace period before marking heartbeat down (seconds)")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func newMonitorsShow() *cobra.Command {
	var asJSON bool
	cmd := command.New("show", "Show monitor details with cert/domain info", "", func(ctx context.Context, args []string) error {
		c, err := client()
		if err != nil {
			return err
		}
		data, err := c.GetMonitor(ctxOrTodo(ctx), args[0])
		if err != nil {
			return err
		}
		io := iostreams.FromContext(ctx)
		if asJSON {
			_, err := io.Out.Write(data)
			return err
		}
		var m struct {
			Name            string `json:"name"`
			Type            string `json:"type"`
			Target          string `json:"target"`
			Enabled         bool   `json:"enabled"`
			IntervalSeconds int    `json:"interval_seconds"`
			TimeoutMs       int    `json:"timeout_ms"`
			PingURL         string `json:"pingUrl"`
			CertStatus      *struct {
				CertExpiresAt       *string  `json:"cert_expires_at"`
				CertDaysRemaining   *int     `json:"cert_days_remaining"`
				CertValid           *bool    `json:"cert_valid"`
				CertIssuer          *string  `json:"cert_issuer"`
				DomainExpiresAt     *string  `json:"domain_expires_at"`
				DomainDaysRemaining *int     `json:"domain_days_remaining"`
				DomainRegistrar     *string  `json:"domain_registrar"`
				DomainRegistryLock  *bool    `json:"domain_registry_lock"`
				DomainNameservers   []string `json:"domain_nameservers"`
			} `json:"certStatus"`
			RecentResults []struct {
				Success        bool     `json:"success"`
				Region         string   `json:"region"`
				StatusCode     *int     `json:"status_code"`
				ResponseTimeMs *float64 `json:"response_time_ms"`
				CheckedAt      string   `json:"checked_at"`
			} `json:"recentResults"`
		}
		if err := json.Unmarshal(data, &m); err != nil {
			return err
		}
		fmt.Fprintf(io.Out, "%s\n", m.Name)
		fmt.Fprintf(io.Out, "type %s  target %s  enabled %v\n", m.Type, m.Target, m.Enabled)
		fmt.Fprintf(io.Out, "interval %ds  timeout %dms\n", m.IntervalSeconds, m.TimeoutMs)
		if m.PingURL != "" {
			fmt.Fprintf(io.Out, "ping  %s\n", m.PingURL)
		}
		if cs := m.CertStatus; cs != nil {
			if cs.CertExpiresAt != nil {
				valid := "INVALID"
				if cs.CertValid != nil && *cs.CertValid {
					valid = "valid"
				}
				fmt.Fprintf(io.Out, "\nSSL: %s, expires %s", valid, *cs.CertExpiresAt)
				if cs.CertDaysRemaining != nil {
					fmt.Fprintf(io.Out, " (%dd left)", *cs.CertDaysRemaining)
				}
				if cs.CertIssuer != nil {
					fmt.Fprintf(io.Out, ", issuer %s", *cs.CertIssuer)
				}
				fmt.Fprintln(io.Out)
			}
			if cs.DomainExpiresAt != nil {
				fmt.Fprintf(io.Out, "Domain: expires %s", *cs.DomainExpiresAt)
				if cs.DomainDaysRemaining != nil {
					fmt.Fprintf(io.Out, " (%dd left)", *cs.DomainDaysRemaining)
				}
				if cs.DomainRegistrar != nil {
					fmt.Fprintf(io.Out, ", registrar %s", *cs.DomainRegistrar)
				}
				fmt.Fprintln(io.Out)
			}
		}
		if len(m.RecentResults) > 0 {
			fmt.Fprintln(io.Out, "\nRecent checks:")
			limit := len(m.RecentResults)
			if limit > 5 {
				limit = 5
			}
			rows := make([][]string, 0, limit)
			for _, r := range m.RecentResults[:limit] {
				state := "OK"
				if !r.Success {
					state = "FAIL"
				}
				code := "—"
				if r.StatusCode != nil {
					code = strconv.Itoa(*r.StatusCode)
				}
				rt := "—"
				if r.ResponseTimeMs != nil {
					rt = fmt.Sprintf("%.0fms", *r.ResponseTimeMs)
				}
				rows = append(rows, []string{state, r.Region, code, rt, r.CheckedAt})
			}
			return render.Table(io.Out, []string{"STATE", "REGION", "CODE", "RT", "AT"}, rows)
		}
		return nil
	})
	cmd.Args = cobra.ExactArgs(1)
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output JSON")
	return cmd
}

func newMonitorsStats() *cobra.Command {
	var hours int
	var asJSON bool
	cmd := command.New("stats", "Show uptime stats", "", func(ctx context.Context, args []string) error {
		c, err := client()
		if err != nil {
			return err
		}
		data, err := c.GetMonitorStats(ctxOrTodo(ctx), args[0], hours)
		if err != nil {
			return err
		}
		io := iostreams.FromContext(ctx)
		if asJSON {
			_, err := io.Out.Write(data)
			return err
		}
		var stats struct {
			Hours         int     `json:"hours"`
			UptimePercent *string `json:"uptimePercent"`
			ByRegion      []struct {
				Region            string   `json:"region"`
				TotalChecks       int      `json:"totalChecks"`
				UptimePercent     string   `json:"uptimePercent"`
				AvgResponseTimeMs *float64 `json:"avgResponseTimeMs"`
			} `json:"byRegion"`
		}
		if err := json.Unmarshal(data, &stats); err != nil {
			return err
		}
		uptime := "—"
		if stats.UptimePercent != nil {
			uptime = *stats.UptimePercent
		}
		fmt.Fprintf(io.Out, "Uptime (%dh): %s%%\n\n", stats.Hours, uptime)
		if len(stats.ByRegion) == 0 {
			return nil
		}
		rows := make([][]string, 0, len(stats.ByRegion))
		for _, r := range stats.ByRegion {
			rt := "—"
			if r.AvgResponseTimeMs != nil {
				rt = fmt.Sprintf("%.0fms", *r.AvgResponseTimeMs)
			}
			rows = append(rows, []string{r.Region, r.UptimePercent + "%", rt, fmt.Sprintf("%d", r.TotalChecks)})
		}
		return render.Table(io.Out, []string{"REGION", "UPTIME", "AVG RT", "CHECKS"}, rows)
	})
	cmd.Args = cobra.ExactArgs(1)
	cmd.Flags().IntVar(&hours, "hours", 24, "Hours to look back")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output JSON")
	return cmd
}

func newMonitorsToggle(verb string, enabled bool) *cobra.Command {
	short := "Enable a monitor"
	msg := "Monitor enabled."
	if !enabled {
		short = "Disable a monitor"
		msg = "Monitor disabled."
	}
	cmd := command.New(verb, short, "", func(ctx context.Context, args []string) error {
		c, err := client()
		if err != nil {
			return err
		}
		if _, err := c.UpdateMonitor(ctxOrTodo(ctx), args[0], map[string]any{"enabled": enabled}); err != nil {
			return err
		}
		fmt.Fprintln(iostreams.FromContext(ctx).Out, msg)
		return nil
	})
	cmd.Args = cobra.ExactArgs(1)
	return cmd
}

func newMonitorsDelete() *cobra.Command {
	cmd := command.New("delete", "Delete a monitor", "", func(ctx context.Context, args []string) error {
		c, err := client()
		if err != nil {
			return err
		}
		if err := c.DeleteMonitor(ctxOrTodo(ctx), args[0]); err != nil {
			return err
		}
		fmt.Fprintln(iostreams.FromContext(ctx).Out, "Monitor deleted.")
		return nil
	})
	cmd.Args = cobra.ExactArgs(1)
	return cmd
}
