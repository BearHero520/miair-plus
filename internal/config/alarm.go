package config

type Alarm struct {
	Rule     string `json:"rule"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	Time     string `json:"time"`
	Timezone string `json:"timezone"`
	Days     int    `json:"days"`
	Speaker  string `json:"speaker"`
	Sound    string `json:"sound"`
	Audio    string `json:"audio"`
	Volume   int    `json:"volume"`
	Minutes  int    `json:"minutes"`
	Enabled  bool   `json:"enabled"`
	LastFire string `json:"last_fire,omitempty"`
	SnoozeAt int64  `json:"snooze_at,omitempty"`
}
