package schema

import "time"

type IndexedTorrent struct {
	Title         string    `json:"title"`
	OriginalTitle string    `json:"original_title"`
	Context       string    `json:"context,omitempty"`
	Details       string    `json:"details"`
	Year          string    `json:"year,omitempty"`
	IMDB          string    `json:"imdb,omitempty"`
	Audio         []Audio   `json:"audio"`
	MagnetLink    string    `json:"magnet_link"`
	Date          time.Time `json:"date"`
	InfoHash      string    `json:"info_hash"`
	Trackers      []string  `json:"trackers,omitempty"`
	Size          string    `json:"size,omitempty"`
	Quality       string    `json:"quality,omitempty"`
	VideoQuality  string    `json:"video_quality,omitempty"`
	AudioQuality  string    `json:"audio_quality,omitempty"`
	Genres        []string  `json:"genres,omitempty"`
	Subtitles     []string  `json:"subtitles,omitempty"`
	Duration      string    `json:"duration,omitempty"`
	Classification string   `json:"classification,omitempty"`
	Files         []File    `json:"files,omitempty"`
	LeechCount    int       `json:"leech_count"`
	SeedCount     int       `json:"seed_count"`
	Similarity    float32   `json:"similarity"`
}

type File struct {
	Path string `json:"path"`
	Size string `json:"size"`
}
