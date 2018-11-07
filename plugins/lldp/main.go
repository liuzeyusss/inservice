package main

import (
	"log"
	"github.com/RackHD/inservice/agent"
)

var binaryName, buildDate, buildUser, commitHash, goVersion, osArch, releaseVersion string

func main() {
	log.Println(binaryName)
	log.Println("  Release version: " + releaseVersion)
	log.Println("  Built On: " + buildDate)
	log.Println("  Build By: " + buildUser)
	log.Println("  Commit Hash: " + commitHash)
	log.Println("  Go version: " + goVersion)
	log.Println("  OS/Arch: " + osArch)

	/*ins, _ := net.Interfaces()
	for _, v := range ins {
		fmt.Println(v.Name)
	}*/

	interfaces := []string{"vEthernet (wifi桥出来的网卡)"}
	lldp, err := NewLLDPPlugin(
		"127.0.0.1",
		80,
		interfaces,
	)
	if err != nil {
		log.Fatalf("Unable to initialize Plugin: %s\n", err)
	}

	p, err := plugins.NewPlugin(lldp)
	if err != nil {
		log.Fatalf("Unable to host Plugin: %s\n", err)
	}

	p.Serve()
}
