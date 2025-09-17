package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/pelletier/go-toml"
)

type ConfigType struct {
	ApiToken      string  `toml:"api_token"`
	Users         []int64 `toml:"users"`
	JournalFolder string  `toml:"journal_folder"`
}

var Cfg ConfigType

func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func journal_filename(unixTime int) string {
	// Get current date
	// return in format YYYY-MM-DD
	t := time.Unix(int64(unixTime), 0)

	// Format as YYYY-MM-DD
	return Cfg.JournalFolder + t.Format("2006-01-02") + ".md"
}

func LoadConfig() {
	data, err := os.ReadFile("config.toml")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	if err := toml.Unmarshal(data, &Cfg); err != nil {
		fmt.Println("Error parsing TOML:", err)
		os.Exit(1)
	}
	if !strings.HasSuffix(Cfg.JournalFolder, "/") {
		Cfg.JournalFolder += "/"
	}
}
