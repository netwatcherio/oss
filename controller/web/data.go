// web/probe_data.go
package web

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kataras/iris/v12"
	"gorm.io/gorm"

	"netwatcher-controller/internal/probe"
)

// Wire this similarly to panelProbes:
// func Register(api iris.Party, pg *gorm.DB, ch *sql.DB) {
//     panelProbes(api, pg)
//     panelProbeData(api, pg, ch)
// }

func panelProbeData(api iris.Party, pg *gorm.DB, ch *sql.DB) {
	base := api.Party("/workspaces/{id:uint}/probe-data")

	// ------------------------------------------
	// GET /workspaces/{id}/network-map
	// Aggregated network topology map for the workspace
	// Query: lookback=<minutes, default 15>
	// ------------------------------------------
	api.Get("/workspaces/{id:uint}/network-map", func(ctx iris.Context) {
		wID := uintParam(ctx, "id")
		lookback := intOrDefault(ctx.URLParam("lookback"), 15)

		mapData, err := probe.GetWorkspaceNetworkMap(ctx.Request().Context(), ch, pg, wID, lookback)
		if err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		_ = ctx.JSON(mapData)
	})

	// ------------------------------------------
	// GET /workspaces/{id}/probe-data/find
	// Flexible finder across ClickHouse with query params mirroring pd.FindParams
	// ------------------------------------------
	base.Get("/find", func(ctx iris.Context) {
		p, bad := readFindParams(ctx)
		if bad != nil {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": bad.Error()})
			return
		}
		rows, err := probe.FindProbeData(ctx.Request().Context(), ch, p)
		if err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		_ = ctx.JSON(NewListResponse(rows))
	})

	// ------------------------------------------
	// GET /workspaces/{id}/probe-data/agents/{agentID}/speedtests
	// Speedtest data for an agent (queries by agent_id + type, NOT probe_id)
	// This works around historical data having incorrect probe_id values
	// Query: limit (default 25)
	// ------------------------------------------
	base.Get("/agents/{agentID:uint}/speedtests", func(ctx iris.Context) {
		agentID := uint64(uintParam(ctx, "agentID"))
		limit := intOrDefault(ctx.URLParam("limit"), 25)

		typ := string(probe.TypeSpeedtest)
		rows, err := probe.FindProbeData(ctx.Request().Context(), ch, probe.FindParams{
			Type:    &typ,
			AgentID: &agentID,
			Limit:   limit,
		})
		if err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		_ = ctx.JSON(NewListResponse(rows))
	})

	// ------------------------------------------
	// GET /workspaces/{id}/probe-data/probes/{probeID}/data
	// Timeseries for one probe (ClickHouse)
	// Query: from, to, limit, asc=true|false, aggregate=<seconds>, type=PING|TRAFFICSIM
	// When aggregate > 0, returns time-bucket averaged data to reduce transfer
	// ------------------------------------------
	base.Get("/probes/{probeID:uint}/data", func(ctx iris.Context) {
		probeID := uint64(uintParam(ctx, "probeID"))

		from, _ := readTime(ctx.URLParam("from"))
		to, _ := readTime(ctx.URLParam("to"))
		limit := intOrDefault(ctx.URLParam("limit"), 0)
		asc := boolOr(ctx.URLParamDefault("asc", ""), false)
		aggregateSec := intOrDefault(ctx.URLParam("aggregate"), 0)
		probeType := ctx.URLParam("type") // "PING" or "TRAFFICSIM"

		var rows []probe.ProbeData
		var err error

		if aggregateSec > 0 && (probeType == "PING" || probeType == "TRAFFICSIM") {
			// Use aggregated query for performance
			rows, err = probe.GetProbeDataAggregated(ctx.Request().Context(), ch, probeID, probeType, from, to, aggregateSec, limit)
			// Log aggregation for debugging
			if err == nil {
				ctx.Application().Logger().Debugf("[ProbeData] Aggregated query: probeID=%d type=%s aggregate=%ds from=%v to=%v -> %d rows",
					probeID, probeType, aggregateSec, from, to, len(rows))
			}
		} else {
			// Standard non-aggregated query
			rows, err = probe.GetProbeDataByProbe(ctx.Request().Context(), ch, probeID, from, to, asc, limit)
			// Log raw query for debugging
			if err == nil && aggregateSec > 0 {
				ctx.Application().Logger().Debugf("[ProbeData] Raw query (type=%s not supported for aggregation): probeID=%d -> %d rows",
					probeType, probeID, len(rows))
			}
		}

		if err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		_ = ctx.JSON(NewListResponse(rows))
	})

	// ------------------------------------------
	// GET /workspaces/{id}/probe-data/latest
	// Latest row by type + reporting agent (and optional probe_id)
	// Query: type=<PING|MTR|...>, agentId=<uint>, probeId=<uint?>
	// ------------------------------------------
	base.Get("/latest", func(ctx iris.Context) {
		typ := ctx.URLParam("type")
		if typ == "" {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "type is required"})
			return
		}
		agentID, ok := parseUint64(ctx.URLParam("agentId"))
		if !ok {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "agentId is required uint"})
			return
		}
		var probeIDPtr *uint64
		if v := ctx.URLParam("probeId"); v != "" {
			if pid, ok := parseUint64(v); ok {
				probeIDPtr = &pid
			} else {
				ctx.StatusCode(http.StatusBadRequest)
				_ = ctx.JSON(iris.Map{"error": "probeId must be uint"})
				return
			}
		}
		row, err := probe.GetLatestByTypeAndAgent(ctx.Request().Context(), ch, typ, agentID, probeIDPtr)
		if err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		if row == nil {
			ctx.StatusCode(http.StatusNotFound)
			return
		}
		_ = ctx.JSON(row)
	})

	// ------------------------------------------
	// GET /workspaces/{id}/probe-data/by-target/data
	// Timeseries for all probes (optionally filtered by type) that hit a literal target (probe_targets.target).
	// Query: target=<host|ip[:port]>, type=<PING|... optional>, limit, from, to, latestOnly (bool)
	// ------------------------------------------
	base.Get("/by-target/data", func(ctx iris.Context) {
		target := strings.TrimSpace(ctx.URLParam("target"))
		if target == "" {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "target is required"})
			return
		}
		var typ *string
		if t := strings.TrimSpace(ctx.URLParam("type")); t != "" {
			typ = &t
		}

		from, _ := readTime(ctx.URLParam("from"))
		to, _ := readTime(ctx.URLParam("to"))
		limit := intOrDefault(ctx.URLParam("limit"), 0)
		latestOnly := boolOr(ctx.URLParamDefault("latestOnly", ""), false)

		// Lookup matching probes from Postgres
		probeIDs, err := findProbeIDsByLiteralTarget(context.TODO(), pg, target, typ)
		if err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		if len(probeIDs) == 0 {
			_ = ctx.JSON(iris.Map{
				"target":   target,
				"probeIds": []uint{},
				"rows":     []any{},
			})
			return
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
				row, err := probe.GetLatest(ctx.Request().Context(), ch, probe.FindParams{ProbeID: uint64Ptr(uint64(pid))})
				if err != nil {
					ctx.StatusCode(http.StatusInternalServerError)
					_ = ctx.JSON(iris.Map{"error": err.Error()})
					return
				}
				out = append(out, bundle{ProbeID: pid, Latest: row})
			} else {
				rows, err := probe.GetProbeDataByProbe(ctx.Request().Context(), ch, uint64(pid), from, to, false, limit)
				if err != nil {
					ctx.StatusCode(http.StatusInternalServerError)
					_ = ctx.JSON(iris.Map{"error": err.Error()})
					return
				}
				out = append(out, bundle{ProbeID: pid, Rows: rows})
			}
		}

		_ = ctx.JSON(iris.Map{
			"target":   target,
			"probeIds": probeIDs,
			"bundles":  out,
		})
	})

	// ------------------------------------------
	// GET /workspaces/{id}/probe-data/probes/{probeID}/similar
	// Discover "like probes" so the UI can combine views:
	//   - same literal target(s)
	//   - same target agent(s) (reverse / inter-agent)
	// Optional query: sameType=true (default true) to restrict to identical probe type.
	// Optional query: includeSelf=false (default false) to exclude current probe.
	// Optional query: latest=true to attach latest datapoint for each similar probe.
	// ------------------------------------------
	base.Get("/probes/{probeID:uint}/similar", func(ctx iris.Context) {
		selfID := uintParam(ctx, "probeID")
		sameType := boolOr(ctx.URLParamDefault("sameType", ""), true)
		includeSelf := boolOr(ctx.URLParamDefault("includeSelf", ""), false)
		withLatest := boolOr(ctx.URLParamDefault("latest", ""), false)

		// Load the reference probe & its targets
		ref, err := probe.GetByID(ctx.Request().Context(), pg, selfID)
		if err != nil || ref == nil {
			if err == nil {
				err = errors.New("probe not found")
			}
			ctx.StatusCode(http.StatusNotFound)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}

		// Gather target literals and target agents from ref
		literals, agentTargets := splitTargets(ref.Targets)

		// Find similar by literal targets
		simLit, err := findProbesByLiteralTargets(context.TODO(), pg, literals, ref.Type, sameType, includeSelf, selfID)
		if err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		// Find similar by target agents
		simAgent, err := findProbesByTargetAgents(context.TODO(), pg, agentTargets, ref.Type, sameType, includeSelf, selfID)
		if err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}

		resp := iris.Map{
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
				row, err := probe.GetLatest(ctx.Request().Context(), ch, probe.FindParams{ProbeID: uint64Ptr(uint64(pid))})
				if err != nil {
					ctx.StatusCode(http.StatusInternalServerError)
					_ = ctx.JSON(iris.Map{"error": err.Error()})
					return
				}
				latest = append(latest, add{ProbeID: pid, Latest: row})
			}
			resp["latest"] = latest
		}

		_ = ctx.JSON(resp)
	})
}

// ---------- helpers (parsing & Postgres lookups) ----------

func readFindParams(ctx iris.Context) (probe.FindParams, error) {
	var p probe.FindParams

	if s := ctx.URLParam("type"); s != "" {
		p.Type = &s
	}
	if v := ctx.URLParam("probeId"); v != "" {
		if x, ok := parseUint64(v); ok {
			p.ProbeID = &x
		} else {
			return p, errors.New("probeId must be uint")
		}
	}
	if v := ctx.URLParam("agentId"); v != "" {
		if x, ok := parseUint64(v); ok {
			p.AgentID = &x
		} else {
			return p, errors.New("agentId must be uint")
		}
	}
	if v := ctx.URLParam("probeAgentId"); v != "" {
		if x, ok := parseUint64(v); ok {
			p.ProbeAgentID = &x
		} else {
			return p, errors.New("probeAgentId must be uint")
		}
	}
	if v := ctx.URLParam("targetAgent"); v != "" {
		if x, ok := parseUint64(v); ok {
			p.TargetAgent = &x
		} else {
			return p, errors.New("targetAgent must be uint")
		}
	}
	if v := ctx.URLParam("targetPrefix"); v != "" {
		p.TargetPrefix = &v
	}
	if v := ctx.URLParam("triggered"); v != "" {
		p.Triggered = boolPtr(v == "1" || strings.EqualFold(v, "true"))
	}
	if t, ok := readTime(ctx.URLParam("from")); ok {
		p.From = t
	}
	if t, ok := readTime(ctx.URLParam("to")); ok {
		p.To = t
	}
	p.Limit = intOrDefault(ctx.URLParam("limit"), 0)
	p.Ascending = boolOr(ctx.URLParamDefault("asc", ""), false)

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
