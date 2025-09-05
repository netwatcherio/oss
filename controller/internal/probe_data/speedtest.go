package probe_data

import "time"

type SpeedTestResult struct {
	TestData  []SpeedTestServer `json:"test_data"`
	Timestamp time.Time         `json:"timestamp" bson:"timestamp"`
}

type SpeedTestServer struct {
	URL          string                `json:"url,omitempty" bson:"url"`
	Lat          string                `json:"lat,omitempty" bson:"lat"`
	Lon          string                `json:"lon,omitempty" bson:"lon"`
	Name         string                `json:"name,omitempty" bson:"name"`
	Country      string                `json:"country,omitempty" bson:"country"`
	Sponsor      string                `json:"sponsor,omitempty" bson:"sponsor"`
	ID           string                `json:"id,omitempty" bson:"id"`
	Host         string                `json:"host,omitempty" bson:"host"`
	Distance     float64               `json:"distance,omitempty" bson:"distance"`
	Latency      time.Duration         `json:"latency,omitempty" bson:"latency"`
	MaxLatency   time.Duration         `json:"max_latency,omitempty" bson:"max_latency"`
	MinLatency   time.Duration         `json:"min_latency,omitempty" bson:"min_latency"`
	Jitter       time.Duration         `json:"jitter,omitempty" bson:"jitter"`
	DLSpeed      SpeedTestByteRate     `json:"dl_speed,omitempty" bson:"dl_speed"`
	ULSpeed      SpeedTestByteRate     `json:"ul_speed,omitempty" bson:"ul_speed"`
	TestDuration SpeedTestTestDuration `json:"test_duration,omitempty" bson:"test_duration"`
	PacketLoss   SpeedTestPLoss        `json:"packet_loss,omitempty" bson:"packet_loss"`
}

type SpeedTestByteRate float64

type SpeedTestTestDuration struct {
	Ping     *time.Duration `json:"ping" bson:"ping"`
	Download *time.Duration `json:"download" bson:"download"`
	Upload   *time.Duration `json:"upload" bson:"upload"`
	Total    *time.Duration `json:"total" bson:"total"`
}

type SpeedTestPLoss struct {
	Sent int `json:"sent" bson:"sent"` // Number of sent packets acknowledged by the remote.
	Dup  int `json:"dup" bson:"dup"`   // Number of duplicate packets acknowledged by the remote.
	Max  int `json:"max" bson:"max"`   // The maximum index value received by the remote.
}
