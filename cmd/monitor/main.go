package main

import (
	"flag"
)

func main() {

	var configPath string
	flag.StringVar(&configPath, "config", "config/config.json", "Path to config file")

	// parse the path to config
	flag.Parse()

	mainLoop(configPath)

}
