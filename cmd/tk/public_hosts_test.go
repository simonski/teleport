package main

import (
	"reflect"
	"testing"
)

func TestPublicHostAliasesAreExplicitBareHostnames(t *testing.T) {
	hosts, err := parsePublicHosts("teleport.engineering, teleport.exe.xyz,TELEPORT.ENGINEERING")
	if err != nil || !reflect.DeepEqual(hosts, []string{"teleport.engineering", "teleport.exe.xyz"}) {
		t.Fatal("invalid aliases", hosts, err)
	}
	for _, value := range []string{"https://teleport.exe.xyz", "teleport.engineering,", "example.com:443", "*.exe.xyz", "bad..host", "-bad.host", "bad-.host", "good.example/route"} {
		if _, err := parsePublicHosts(value); err == nil {
			t.Fatal("unsafe hostname accepted", value)
		}
	}
}
