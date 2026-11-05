package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// session is what GoFolio remembers between launches.
type session struct {
	Vault     string       `json:"vault"`
	Recents   []string     `json:"recents,omitempty"`
	Open      []sessionTab `json:"open,omitempty"`
	Active    int          `json:"active"`
	Sidebar   float32      `json:"sidebar"`
	ShowSide  bool         `json:"show_sidebar"`
	Collapsed bool         `json:"collapsed"`
	Backlinks bool         `json:"backlinks"`
	FullText  bool         `json:"full_text"`

	// Settings: defaults applied to newly opened notes. ModeSet records that
	// the user chose DefaultMode; until then new notes open in Live.
	DefaultMode int  `json:"default_mode"`
	ModeSet     bool `json:"mode_set"`
	Spellcheck  bool `json:"spellcheck"`

	// vaults the user added by hand, shown in the Vaults window.
	Pinned []string `json:"pinned,omitempty"`

	// the graph view's display and force settings, remembered like Obsidian's.
	Graph *graphSettings `json:"graph,omitempty"`
}

// graphSettings is the persisted state of the graph view's settings panel.
type graphSettings struct {
	ShowLabels  bool    `json:"show_labels"`
	ShowOrphans bool    `json:"show_orphans"`
	Arrows      bool    `json:"arrows"`
	TextFade    float64 `json:"text_fade"`
	NodeScale   float64 `json:"node_scale"`
	LinkWidth   float64 `json:"link_width"`
	Center      float64 `json:"center"`
	Repel       float64 `json:"repel"`
	Link        float64 `json:"link"`
	LinkRange   float64 `json:"link_range"`
}

type sessionTab struct {
	Path string `json:"path"`
	Mode int    `json:"mode"`
}

func sessionPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "GoFolio", "state.json")
}

func loadSession() session {
	s := session{Sidebar: 260, ShowSide: true, Backlinks: true}
	path := sessionPath()
	if path == "" {
		return s
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return session{Sidebar: 260, ShowSide: true, Backlinks: true}
	}
	if s.Sidebar < 140 {
		s.Sidebar = 260
	}
	return s
}

func saveSession(s session) error {
	path := sessionPath()
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, append(b, '\n'))
}

// remember adds dir to the front of the recent vaults.
func remember(recents []string, dir string) []string {
	out := []string{dir}
	for _, r := range recents {
		if r == dir {
			continue
		}
		out = append(out, r)
		if len(out) == 8 {
			break
		}
	}
	return out
}
