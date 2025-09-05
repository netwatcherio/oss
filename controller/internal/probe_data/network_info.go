package probe_data

import "time"

type NetworkInfoResult struct {
	LocalAddress     string    `json:"local_address" bson:"local_address"`
	DefaultGateway   string    `json:"default_gateway" bson:"default_gateway"`
	PublicAddress    string    `json:"public_address" bson:"public_address"`
	InternetProvider string    `json:"internet_provider" bson:"internet_provider"`
	Lat              string    `json:"lat" bson:"lat"`
	Long             string    `json:"long" bson:"long"`
	Timestamp        time.Time `json:"timestamp" bson:"timestamp"`
}
