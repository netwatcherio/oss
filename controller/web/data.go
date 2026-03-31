// web/probe_data.go
package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"netwatcher-controller/internal/probe"
)

func panelProbeData(api fiber.Router, pg *gorm.DB, ch *sql.DB) {
	base := api.Group("/workspaces/:id/probe-data")

	// ------------------------------------------
	// GET /workspaces/:id/network-map
	// Aggregated network topology map for the workspace
	// Query: lookback=<minutes, default 15>
	// ------------------------------------------
	api.Get("/workspaces/:id/network-map", func(c *fiber.Ctx) error {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[network-map] PANIC: %v", r)
				_ = c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "internal error"})
			}
		}()

		wID := uintParam(c, "id")
		lookback := intOrDefault(c.Query("lookback"), 15)

		mapData, err := probe.GetWorkspaceNetworkMap(c.UserContext(), ch, pg, wID, lookback)
		if err != nil {
			log.Printf("[network-map] workspace=%d error: %v", wID, err)
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		// Explicitly marshal to check for JSON errors
		jsonBytes, err := json.Marshal(mapData)
		if err != nil {
			log.Printf("[network-map] JSON marshal error: %v", err)
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "json serialization failed"})
		}

		c.Set("Content-Type", "application/json")
		return c.Send(jsonBytes)
	})

	// ------------------------------------------
	// GET /workspaces/:id/connectivity-matrix
	// Aggregated connectivity matrix for the workspace
	// Query: lookback=<minutes, default 15>
	// ------------------------------------------
	api.Get("/workspaces/:id/connectivity-matrix", func(c *fiber.Ctx) error {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[connectivity-matrix] PANIC: %v", r)
				_ = c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "internal error"})
			}
		}()

		wID := uintParam(c, "id")
		lookback := intOrDefault(c.Query("lookback"), 15)

		matrix, err := probe.GetWorkspaceConnectivityMatrix(c.UserContext(), ch, pg, wID, lookback)
		if err != nil {
			log.Printf("[connectivity-matrix] workspace=%d error: %v", wID, err)
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		// Explicitly marshal to check for JSON errors
		jsonBytes, err := json.Marshal(matrix)
		if err != nil {
			log.Printf("[connectivity-matrix] JSON marshal error: %v", err)
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "json serialization failed"})
		}

		c.Set("Content-Type", "application/json")
		return c.Send(jsonBytes)
	})

	// ------------------------------------------
	// GET /workspaces/:id/probe-data/find
	// Flexible finder across ClickHouse with query params mirroring pd.FindParams
	// ------------------------------------------
	base.Get("/find", func(c *fiber.Ctx) error {
		p, bad := readFindParams(c)
		if bad != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": bad.Error()})
		}
		rows, err := probe.FindProbeData(c.UserContext(), ch, p)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(NewListResponse(rows))
	})

	// ------------------------------------------
	// GET /workspaces/:id/probe-data/agents/:agentID/speedtests
	// Speedtest data for an agent (queries by agent_id + type, NOT probe_id)
	// This works around historical data having incorrect probe_id values
	// Query: limit (default 25)
	// ------------------------------------------
	base.Get("/agents/:agentID/speedtests", func(c *fiber.Ctx) error {
		agentID := uint64(uintParam(c, "agentID"))
		limit := intOrDefault(c.Query("limit"), 25)

		typ := string(probe.TypeSpeedtest)
		rows, err := probe.FindProbeData(c.UserContext(), ch, probe.FindParams{
			Type:    &typ,
			AgentID: &agentID,
			Limit:   limit,
		})
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(NewListResponse(rows))
	})

	// ------------------------------------------
	// GET /workspaces/:id/probe-data/probes/:probeID/data
	// Timeseries for one probe (ClickHouse)
	// Query: from, to, limit, asc=true|false, aggregate=<seconds>, type=PING|TRAFFICSIM, agentId=<uint>
	// When aggregate > 0, returns time-bucket averaged data to reduce transfer
	// When agentId is specified, filters by the reporting agent (for AGENT probes with bidirectional data)
	// ------------------------------------------
	base.Get("/probes/:probeID/data", func(c *fiber.Ctx) error {
		probeID := uint64(uintParam(c, "probeID"))

		// Optional agentId filter - used for AGENT probes to filter by specific reporter
		var agentID *uint64
		if v := c.Query("agentId"); v != "" {
			if x, ok := parseUint64(v); ok {
				agentID = &x
			}
		}

		from, _ := readTime(c.Query("from"))
		to, _ := readTime(c.Query("to"))
		limit := intOrDefault(c.Query("limit"), 0)
		asc := boolOr(c.Query("asc", ""), false)
		aggregateSec := intOrDefault(c.Query("aggregate"), 0)
		probeType := c.Query("type") // "PING" or "TRAFFICSIM"

		var rows []probe.ProbeData
		var err error

		if aggregateSec > 0 && (probeType == "PING" || probeType == "TRAFFICSIM" || probeType == "MTR") {
			// Use aggregated query for performance
			rows, err = probe.GetProbeDataAggregated(c.UserContext(), ch, probeID, agentID, probeType, from, to, aggregateSec, limit)
			// Log aggregation for debugging
			if err == nil {
				log.Printf("[ProbeData] Aggregated query: probeID=%d agentID=%v type=%s aggregate=%ds from=%v to=%v -> %d rows",
					probeID, agentID, probeType, aggregateSec, from, to, len(rows))
			}
		} else {
			// Standard non-aggregated query
			rows, err = probe.GetProbeDataByProbe(c.UserContext(), ch, probeID, agentID, from, to, asc, limit)
			// Log raw query for debugging
			if err == nil && aggregateSec > 0 {
				log.Printf("[ProbeData] Raw query (type=%s not supported for aggregation): probeID=%d -> %d rows",
					probeType, probeID, len(rows))
			}
			// Post-filter by type if specified
			if err == nil && probeType != "" {
				filtered := make([]probe.ProbeData, 0, len(rows))
				for _, r := range rows {
					if string(r.Type) == probeType {
						filtered = append(filtered, r)
					}
				}
				rows = filtered
			}
		}

		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(NewListResponse(rows))
	})

	// ------------------------------------------
	// GET /workspaces/:id/probe-data/latest
	// Latest row by type + reporting agent (and optional probe_id)
	// Query: type=<PING|MTR|...>, agentId=<uint>, probeId=<uint?>
	// ------------------------------------------
	base.Get("/latest", func(c *fiber.Ctx) error {
		typ := c.Query("type")
		if typ == "" {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "type is required"})
		}
		agentID, ok := parseUint64(c.Query("agentId"))
		if !ok {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "agentId is required uint"})
		}
		var probeIDPtr *uint64
		if v := c.Query("probeId"); v != "" {
			if pid, ok := parseUint64(v); ok {
				probeIDPtr = &pid
			} else {
				return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "probeId must be uint"})
			}
		}
		row, err := probe.GetLatestByTypeAndAgent(c.UserContext(), ch, typ, agentID, probeIDPtr)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		if row == nil {
			return c.SendStatus(http.StatusNotFound)
		}
		return c.JSON(row)
	})

	// ------------------------------------------
	// GET /workspaces/:id/probe-data/by-target/data
	// Timeseries for all probes (optionally filtered by type) that hit a literal target (probe_targets.target).
	// Query: target=<host|ip[:port]>, type=<PING|... optional>, limit, from, to, latestOnly (bool)
	// ------------------------------------------
	base.Get("/by-target/data", func(c *fiber.Ctx) error {
		target := strings.TrimSpace(c.Query("target"))
		if target == "" {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "target is required"})
		}
		var typ *string
		if t := strings.TrimSpace(c.Query("type")); t != "" {
			typ = &t
		}

		from, _ := readTime(c.Query("from"))
		to, _ := readTime(c.Query("to"))
		limit := intOrDefault(c.Query("limit"), 0)
		latestOnly := boolOr(c.Query("latestOnly", ""), false)

		// Lookup matching probes from Postgres
		probeIDs, err := findProbeIDsByLiteralTarget(context.TODO(), pg, target, typ)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		if len(probeIDs) == 0 {
			return c.JSON(fiber.Map{
				"target":   target,
				"probeIds": []uint{},
				"rows":     []any{},
			})
		}

		// Fetch either latest points or timeseries for each probe_id
		type bundle struct {
			ProbeID uint              `json:"probe_id"`
			Latest  *probe.ProbeData  `json:"latest,omitempty"`
			Rows    []probe.ProbeData `json:"rows,omitempty"`
		}
		out := make([]bundle, 0, len(probeIDs))
		for _, pid := range probeIDs {
			if latestOnly {
				row, err := probe.GetLatest(c.UserContext(), ch, probe.FindParams{ProbeID: uint64Ptr(uint64(pid))})
				if err != nil {
					return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
				}
				out = append(out, bundle{ProbeID: pid, Latest: row})
			} else {
				rows, err := probe.GetProbeDataByProbe(c.UserContext(), ch, uint64(pid), nil, from, to, false, limit)
				if err != nil {
					return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
				}
				out = append(out, bundle{ProbeID: pid, Rows: rows})
			}
		}

		return c.JSON(fiber.Map{
			"target":   target,
			"probeIds": probeIDs,
			"bundles":  out,
		})
	})

	// ------------------------------------------
	// GET /workspaces/:id/probe-data/probes/:probeID/similar
	// Discover "like probes" so the UI can combine views:
	//   - same literal target(s)
	//   - same target agent(s) (reverse / inter-agent)
	// Optional query: sameType=true (default true) to restrict to identical probe type.
	// Optional query: includeSelf=false (default false) to exclude current probe.
	// Optional query: latest=true to attach latest datapoint for each similar probe.
	// ------------------------------------------
	base.Get("/probes/:probeID/similar", func(c *fiber.Ctx) error {
		selfID := uintParam(c, "probeID")
		sameType := boolOr(c.Query("sameType", ""), true)
		includeSelf := boolOr(c.Query("includeSelf", ""), false)
		withLatest := boolOr(c.Query("latest", ""), false)

		// Load the reference probe & its targets
		ref, err := probe.GetByID(c.UserContext(), pg, selfID)
		if err != nil || ref == nil {
			if err == nil {
				err = errors.New("probe not found")
			}
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}

		// Gather target literals and target agents from ref
		literals, agentTargets := splitTargets(ref.Targets)

		// Find similar by literal targets
		simLit, err := findProbesByLiteralTargets(context.TODO(), pg, literals, ref.Type, sameType, includeSelf, selfID)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		// Find similar by target agents
		simAgent, err := findProbesByTargetAgents(context.TODO(), pg, agentTargets, ref.Type, sameType, includeSelf, selfID)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		resp := fiber.Map{
			"probe":               ref,
			"similar_by_target":   simLit,
			"similar_by_agent_id": simAgent,
		}

		// Optionally attach latest datapoints for each similar probe (by probe_id only)
		if withLatest {
			type add struct {
				ProbeID uint             `json:"probe_id"`
				Latest  *probe.ProbeData `json:"latest"`
			}
			var ids []uint
			for _, p := range simLit {
				ids = append(ids, p.ID)
			}
			for _, p := range simAgent {
				ids = append(ids, p.ID)
			}
			ids = uniqueUint(ids)

			latest := make([]add, 0, len(ids))
			for _, pid := range ids {
				row, err := probe.GetLatest(c.UserContext(), ch, probe.FindParams{ProbeID: uint64Ptr(uint64(pid))})
				if err != nil {
					return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
				}
				latest = append(latest, add{ProbeID: pid, Latest: row})
			}
			resp["latest"] = latest
		}

		return c.JSON(resp)
	})

	// ------------------------------------------
	// GET /workspaces/:id/probe-data/agents/:agentID/dns
	// DNS dashboard data - returns DNS probe results grouped by target hostname
	// Query: limit (default 50), lookback (minutes, default 60)
	// ------------------------------------------
	base.Get("/agents/:agentID/dns", func(c *fiber.Ctx) error {
		agentID := uint64(uintParam(c, "agentID"))
		limit := intOrDefault(c.Query("limit"), 500)
		lookbackMin := intOrDefault(c.Query("lookback"), 60)

		from := time.Now().UTC().Add(-time.Duration(lookbackMin) * time.Minute)

		typ := string(probe.TypeDNS)
		rows, err := probe.FindProbeData(c.UserContext(), ch, probe.FindParams{
			Type:    &typ,
			AgentID: &agentID,
			From:    from,
			Limit:   limit,
		})
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		// Group results by target hostname
		type dnsGroupEntry struct {
			CreatedAt time.Time       `json:"created_at"`
			ProbeID   uint            `json:"probe_id"`
			Payload   json.RawMessage `json:"payload"`
			Target    string          `json:"target"`
		}
		type dnsGroup struct {
			Target  string          `json:"target"`
			Count   int             `json:"count"`
			Entries []dnsGroupEntry `json:"entries"`
		}

		groupMap := make(map[string]*dnsGroup)
		var groupOrder []string

		for _, row := range rows {
			target := row.Target
			if target == "" {
				var p struct {
					Target string `json:"target"`
				}
				if err := json.Unmarshal(row.Payload, &p); err == nil && p.Target != "" {
					target = p.Target
				} else {
					target = "unknown"
				}
			}

			g, exists := groupMap[target]
			if !exists {
				g = &dnsGroup{Target: target}
				groupMap[target] = g
				groupOrder = append(groupOrder, target)
			}
			g.Count++
			g.Entries = append(g.Entries, dnsGroupEntry{
				CreatedAt: row.CreatedAt,
				ProbeID:   row.ProbeID,
				Payload:   row.Payload,
				Target:    target,
			})
		}

		groups := make([]dnsGroup, 0, len(groupOrder))
		for _, key := range groupOrder {
			groups = append(groups, *groupMap[key])
		}

		return c.JSON(fiber.Map{
			"agent_id": agentID,
			"total":    len(rows),
			"groups":   groups,
			"lookback": lookbackMin,
		})
	})
}

// ---------- helpers (parsing & Postgres lookups) ----------

func readFindParams(c *fiber.Ctx) (probe.FindParams, error) {
	var p probe.FindParams

	if s := c.Query("type"); s != "" {
		probeType := probe.Type(s)
		if !probeType.Valid() {
			return p, errors.New("type must be a valid probe type")
		}
		p.Type = &s
	}
	if v := c.Query("probeId"); v != "" {
		if x, ok := parseUint64(v); ok {
			p.ProbeID = &x
		} else {
			return p, errors.New("probeId must be uint")
		}
	}
	if v := c.Query("agentId"); v != "" {
		if x, ok := parseUint64(v); ok {
			p.AgentID = &x
		} else {
			return p, errors.New("agentId must be uint")
		}
	}
	if v := c.Query("probeAgentId"); v != "" {
		if x, ok := parseUint64(v); ok {
			p.ProbeAgentID = &x
		} else {
			return p, errors.New("probeAgentId must be uint")
		}
	}
	if v := c.Query("targetAgent"); v != "" {
		if x, ok := parseUint64(v); ok {
			p.TargetAgent = &x
		} else {
			return p, errors.New("targetAgent must be uint")
		}
	}
	if v := c.Query("targetPrefix"); v != "" {
		p.TargetPrefix = &v
	}
	if v := c.Query("triggered"); v != "" {
		p.Triggered = boolPtr(v == "1" || strings.EqualFold(v, "true"))
	}
	if t, ok := readTime(c.Query("from")); ok {
		p.From = t
	}
	if t, ok := readTime(c.Query("to")); ok {
		p.To = t
	}
	p.Limit = intOrDefault(c.Query("limit"), 0)
	p.Ascending = boolOr(c.Query("asc", ""), false)

	return p, nil
}

func uint64Ptr(u uint64) *uint64 { return &u }

func parseUint64(v string) (uint64, bool) {
	x, err := strconv.ParseUint(v, 10, 64)
	return x, err == nil
}

func intOrDefault(v string, def int) int {
	if v == "" {
		return def
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return i
}

func boolOr(val string, def bool) bool {
	if val == "" {
		return def
	}
	switch strings.ToLower(val) {
	case "1", "true", "t", "yes", "y":
		return true
	case "0", "false", "f", "no", "n":
		return false
	default:
		return def
	}
}

func boolPtr(b bool) *bool { return &b }

// parse RFC3339 or unix seconds; empty -> (zero,false)
func readTime(v string) (time.Time, bool) {
	if v == "" {
		return time.Time{}, false
	}
	// try RFC3339
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, true
	}
	// try unix seconds
	if s, err := strconv.ParseInt(v, 10, 64); err == nil && s > 0 {
		return time.Unix(s, 0).UTC(), true
	}
	return time.Time{}, false
}

// split Targets into literal host strings and target agent IDs
func splitTargets(ts []probe.Target) (literals []string, agentIDs []uint) {
	for _, t := range ts {
		if t.AgentID != nil {
			agentIDs = append(agentIDs, *t.AgentID)
		} else if s := strings.TrimSpace(t.Target); s != "" {
			literals = append(literals, s)
		}
	}
	return
}

func uniqueUint(in []uint) []uint {
	seen := make(map[uint]struct{}, len(in))
	var out []uint
	for _, v := range in {
		if _, ok := seen[v]; !ok {
			seen[v] = struct{}{}
			out = append(out, v)
		}
	}
	return out
}

// ----- Postgres lookups for "like probes" -----

// findProbeIDsByLiteralTarget finds probe IDs that have a specific literal target;
// if typ != nil, restrict to probes.type = *typ.
func findProbeIDsByLiteralTarget(ctxCtx context.Context, pg *gorm.DB, target string, typ *string) ([]uint, error) {
	ctx := ctxCtx
	var ids []uint
	q := pg.WithContext(ctx).
		Table("probe_targets AS t").
		Select("t.probe_id").
		Joins("JOIN probes p ON p.id = t.probe_id").
		Where("t.agent_id IS NULL AND t.target = ?", target)
	if typ != nil && *typ != "" {
		q = q.Where("p.type = ?", *typ)
	}
	if err := q.Find(&ids).Error; err != nil {
		return nil, err
	}
	return uniqueUint(ids), nil
}

func findProbesByLiteralTargets(ctxCtx context.Context, pg *gorm.DB, targets []string, refType probe.Type, sameType, includeSelf bool, selfID uint) ([]probe.Probe, error) {
	ctx := ctxCtx
	if len(targets) == 0 {
		return []probe.Probe{}, nil
	}
	q := pg.WithContext(ctx).Model(&probe.Probe{}).
		Preload("Targets").
		Joins("JOIN probe_targets t ON t.probe_id = probes.id").
		Where("t.agent_id IS NULL AND t.target IN ?", targets)
	if sameType {
		q = q.Where("probes.type = ?", refType)
	}
	if !includeSelf {
		q = q.Where("probes.id <> ?", selfID)
	}
	var out []probe.Probe
	if err := q.Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func findProbesByTargetAgents(ctxCtx context.Context, pg *gorm.DB, agentIDs []uint, refType probe.Type, sameType, includeSelf bool, selfID uint) ([]probe.Probe, error) {
	ctx := ctxCtx
	if len(agentIDs) == 0 {
		return []probe.Probe{}, nil
	}
	q := pg.WithContext(ctx).Model(&probe.Probe{}).
		Preload("Targets").
		Joins("JOIN probe_targets t ON t.probe_id = probes.id").
		Where("t.agent_id IN ?", agentIDs)
	if sameType {
		q = q.Where("probes.type = ?", refType)
	}
	if !includeSelf {
		q = q.Where("probes.id <> ?", selfID)
	}
	var out []probe.Probe
	if err := q.Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}
