package probe_data

import (
	"context"
	"database/sql"
	log "github.com/sirupsen/logrus"
	"netwatcher-controller/internal/probe"
	"time"
)

func initSysInfo(db *sql.DB) {
	Register(NewHandler[sysInfoPayload](
		probe.TypeSysInfo,
		func(p sysInfoPayload) error {
			return nil
		},
		func(ctx context.Context, data ProbeData, p sysInfoPayload) error {
			if err := SaveRecordCH(ctx, db, data, string(probe.TypeSysInfo), p); err != nil {
				log.WithError(err).Error("save sysinfo record (CH)")
				return err
			}

			// Store to DB / compute / alert as needed:
			log.Printf("[sysinfo] id=%d hostname=%s timezone=%s timestamp=%s",
				data.ID, p.HostInfo.Hostname, p.HostInfo.Timezone, p.Timestamp)
			return nil
		},
	))
}

type sysInfoPayload struct {
	HostInfo   SystemHostInfo       `json:"hostInfo" bson:"hostInfo"`
	MemoryInfo SystemHostMemoryInfo `json:"memoryInfo" bson:"memoryInfo"`
	CPUTimes   SystemCPUTimes       `json:"CPUTimes" bson:"CPUTimes"`
	Timestamp  time.Time            `json:"timestamp" bson:"timestamp"`
}

type SystemCPUTimes struct {
	User    time.Duration `json:"user" bson:"user"`
	System  time.Duration `json:"system" bson:"system"`
	Idle    time.Duration `json:"idle,omitempty" bson:"idle"`
	IOWait  time.Duration `json:"iowait,omitempty" bson:"IOWait"`
	IRQ     time.Duration `json:"irq,omitempty" bson:"IRQ"`
	Nice    time.Duration `json:"nice,omitempty" bson:"nice"`
	SoftIRQ time.Duration `json:"soft_irq,omitempty" bson:"softIRQ"`
	Steal   time.Duration `json:"steal,omitempty" bson:"steal"`
}

type SystemHostInfo struct {
	Architecture      string       `json:"architecture" bson:"architecture"`
	BootTime          time.Time    `json:"boot_time" bson:"bootTime"`
	Containerized     *bool        `json:"containerized,omitempty" bson:"containerized"`
	Hostname          string       `json:"name" bson:"hostname"`
	IPs               []string     `json:"ip,omitempty" bson:"IPs"`
	KernelVersion     string       `json:"kernel_version" bson:"kernelVersion"`
	MACs              []string     `json:"mac" bson:"MACs"`
	OS                SystemOSInfo `json:"os" bson:"OS"`
	Timezone          string       `json:"timezone" bson:"timezone"`
	TimezoneOffsetSec int          `json:"timezone_offset_sec" bson:"timezoneOffsetSec"`
	UniqueID          string       `json:"id,omitempty" bson:"uniqueID"`
}

type SystemOSInfo struct {
	Type     string `json:"type" bson:"type"`
	Family   string `json:"family" bson:"family"`
	Platform string `json:"platform" bson:"platform"`
	Name     string `json:"name" bson:"name"`
	Version  string `json:"version" bson:"version"`
	Major    int    `json:"major" bson:"major"`
	Minor    int    `json:"minor" bson:"minor"`
	Patch    int    `json:"patch" bson:"patch"`
	Build    string `json:"build,omitempty" bson:"build"`
	Codename string `json:"codename,omitempty" bson:"codename"`
}

// HostMemoryInfo (all values are specified in bytes).
type SystemHostMemoryInfo struct {
	Total        uint64            `json:"total_bytes" bson:"total"`                // Total physical memory.
	Used         uint64            `json:"used_bytes" bson:"used"`                  // Total - Free
	Available    uint64            `json:"available_bytes" bson:"available"`        // Amount of memory available without swapping.
	Free         uint64            `json:"free_bytes" bson:"free"`                  // Amount of memory not used by the system.
	VirtualTotal uint64            `json:"virtual_total_bytes" bson:"virtualTotal"` // Total virtual memory.
	VirtualUsed  uint64            `json:"virtual_used_bytes" bson:"virtualUsed"`   // VirtualTotal - VirtualFree
	VirtualFree  uint64            `json:"virtual_free_bytes" bson:"virtualFree"`   // Virtual memory that is not used.
	Metrics      map[string]uint64 `json:"raw,omitempty" bson:"metrics"`            // Other memory related metrics.
}
