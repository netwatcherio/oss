package web

import (
	"errors"
	"github.com/kataras/iris/v12"
	"gorm.io/gorm"
	"net/http"
	"netwatcher-controller/internal/agent"
	"strconv"
	"strings"
	"time"

	_ "github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/websocket"
	log "github.com/sirupsen/logrus"
)

// Expected headers for WS:
//   X-Workspace-ID: <uint>
//   X-Agent-ID:     <uint>
//   X-Agent-PSK:    <string>
//
// If valid, we attach agent/workspace IDs to the Iris context for use in events.

func addWebSocketServer(app *iris.Application, db *gorm.DB) error {
	websocketServer := websocket.New(
		websocket.DefaultGorillaUpgrader,
		getWebsocketEvents(app, db),
	)

	// Authenticate connection via PSK
	websocketServer.OnConnect = func(c *websocket.Conn) error {
		ctx := websocket.GetContext(c)

		wsIDStr := ctx.GetHeader("X-Workspace-ID")
		agIDStr := ctx.GetHeader("X-Agent-ID")
		psk := ctx.GetHeader("X-Agent-PSK")

		if wsIDStr == "" || agIDStr == "" || psk == "" {
			ctx.StatusCode(http.StatusUnauthorized)
			return errors.New("unauthorized: missing X-Workspace-ID / X-Agent-ID / X-Agent-PSK")
		}

		wsID64, err := strconv.ParseUint(strings.TrimSpace(wsIDStr), 10, 64)
		if err != nil {
			ctx.StatusCode(http.StatusUnauthorized)
			return errors.New("unauthorized: invalid workspace id")
		}
		agID64, err := strconv.ParseUint(strings.TrimSpace(agIDStr), 10, 64)
		if err != nil {
			ctx.StatusCode(http.StatusUnauthorized)
			return errors.New("unauthorized: invalid agent id")
		}

		a, err := agent.AuthenticateWithPSK(ctx, db, uint(wsID64), uint(agID64), psk)
		if err != nil {
			ctx.StatusCode(http.StatusUnauthorized)
			return errors.New("unauthorized: invalid psk")
		}

		// Mark agent seen
		_ = agent.UpdateAgentSeen(ctx, db, a.ID, time.Now())

		// Stash IDs into the Iris context so namespace handlers can fetch them
		ctx.Values().Set("agent_id", a.ID)
		ctx.Values().Set("workspace_id", a.WorkspaceID)

		log.Infof("WS auth ok — agent %d (ws %d) connected as %s", a.ID, a.WorkspaceID, c.ID())
		return nil
	}

	app.Get("ws", websocket.Handler(websocketServer))
	return nil
}

func getWebsocketEvents(app *iris.Application, db *gorm.DB) websocket.Namespaces {
	serverEvents := websocket.Namespaces{
		"agent": websocket.Events{
			websocket.OnNamespaceConnected: func(nsConn *websocket.NSConn, msg websocket.Message) error {
				ctx := websocket.GetContext(nsConn.Conn)
				aid, _ := ctx.Values().GetUint("agent_id")
				wsid, _ := ctx.Values().GetUint("workspace_id")
				log.Infof("[%s] connected to namespace [%s] (agent=%d ws=%d)", nsConn, msg.Namespace, aid, wsid)
				return nil
			},
			websocket.OnNamespaceDisconnect: func(nsConn *websocket.NSConn, msg websocket.Message) error {
				log.Infof("[%s] disconnected from namespace [%s]", nsConn, msg.Namespace)
				return nil
			},

			"probe_get": func(nsConn *websocket.NSConn, msg websocket.Message) error {
				ctx := websocket.GetContext(nsConn.Conn)
				aid, ok := ctx.Values().GetUint("agent_id")
				if (ok != nil) || (aid == 0) {
					return errors.New("unauthorized: no agent in context")
				}

				// Load and update agent
				a, err := agent.GetAgentByID(ctx, db, uint(aid))
				if err != nil {
					return err
				}
				if err := agent.UpdateAgentSeen(ctx, db, a.ID, time.Now()); err != nil {
					log.Error(err)
				}

				// Fetch probes for this agent
				// NOTE: Adjust your Probe struct if needed; this mirrors your previous logic
				/*probe := agent.Probe{Agent: a.ID}
				probes, err := probe.GetAllProbesForAgent(r.DB)
				if err != nil {
					log.Errorf("probe_get: %v", err)
				}*/

				/*payload, err := json.Marshal(probes)
				if err != nil {
					return err
				}*/

				// Important: nsConn.Emit returns bool; do not treat as error
				nsConn.Emit("probe_get", []byte("bing bong"))
				return nil
			},

			/*"probe_post": func(nsConn *websocket.NSConn, msg websocket.Message) error {
				ctx := websocket.GetContext(nsConn.Conn)
				aid, ok := ctx.Values().GetUint("agent_id")
				if !ok || aid == 0 {
					return errors.New("unauthorized: no agent in context")
				}

				// Ensure agent exists
				a, err := agent.GetAgentByID(ctx, r.DB, uint(aid))
				if err != nil {
					return err
				}

				// Unmarshal posted probe data and push to channel
				data := agent.ProbeData{}
				if err := json.Unmarshal(msg.Body, &data); err != nil {
					log.Error(err)
					return err
				}

				// propagate
				r.ProbeDataChan <- data
				return nil
			},*/
		},
	}

	return serverEvents
}
