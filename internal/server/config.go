package server

import "time"

type Config struct {
	ListenAddress        string        `json:"listen_address"`
	MaxPacketBytes       int           `json:"max_packet_bytes"`
	MaxConnections       int           `json:"max_connections"`
	MaxRooms             int           `json:"max_rooms"`
	MaxMessagesPerSecond int           `json:"max_messages_per_second"`
	WriteQueueBytes      int64         `json:"write_queue_bytes"`
	HeartbeatInterval    time.Duration `json:"-"`
	HeartbeatTimeout     time.Duration `json:"-"`
	HeartbeatIntervalMS  int           `json:"heartbeat_interval_ms"`
	HeartbeatTimeoutMS   int           `json:"heartbeat_timeout_ms"`
}

func DefaultConfig() Config {
	return Config{
		ListenAddress:        ":24555",
		MaxPacketBytes:       16 * 1024 * 1024,
		MaxConnections:       1024,
		MaxRooms:             256,
		MaxMessagesPerSecond: 200,
		WriteQueueBytes:      8 * 1024 * 1024,
		HeartbeatInterval:    2 * time.Second,
		HeartbeatTimeout:     10 * time.Second,
		HeartbeatIntervalMS:  2000,
		HeartbeatTimeoutMS:   10000,
	}
}

func (c *Config) normalize() {
	defaults := DefaultConfig()
	if c.ListenAddress == "" {
		c.ListenAddress = defaults.ListenAddress
	}
	if c.MaxPacketBytes <= 0 {
		c.MaxPacketBytes = defaults.MaxPacketBytes
	}
	if c.MaxConnections <= 0 {
		c.MaxConnections = defaults.MaxConnections
	}
	if c.MaxRooms <= 0 {
		c.MaxRooms = defaults.MaxRooms
	}
	if c.MaxMessagesPerSecond <= 0 {
		c.MaxMessagesPerSecond = defaults.MaxMessagesPerSecond
	}
	if c.WriteQueueBytes <= 0 {
		c.WriteQueueBytes = defaults.WriteQueueBytes
	}
	if c.HeartbeatIntervalMS > 0 {
		c.HeartbeatInterval = time.Duration(c.HeartbeatIntervalMS) * time.Millisecond
	}
	if c.HeartbeatTimeoutMS > 0 {
		c.HeartbeatTimeout = time.Duration(c.HeartbeatTimeoutMS) * time.Millisecond
	}
	if c.HeartbeatInterval <= 0 {
		c.HeartbeatInterval = defaults.HeartbeatInterval
	}
	if c.HeartbeatTimeout <= 0 {
		c.HeartbeatTimeout = defaults.HeartbeatTimeout
	}
}
