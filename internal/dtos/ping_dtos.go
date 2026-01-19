package dtos

type SystemStats struct {
	MemoryUsageMB uint64 `json:"memory_usage_mb" example:"24"`
	Goroutines    int    `json:"goroutines" example:"8"`
}
type PingResponse struct {
	Status      string      `json:"status" example:"up"`
	Environment string      `json:"environment" example:"development"`
	Version     string      `json:"version" example:"1.0.0"`
	Uptime      string      `json:"uptime" example:"20s"`
	System      SystemStats `json:"system"`
	Timestamp   string      `json:"timestamp" example:"2026-01-14T14:20:42+05:45"`
}
