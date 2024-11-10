// Package main ...
package main

import (
	"fmt"

	"github.com/yontaruron/rod/lib/launcher"
	"github.com/yontaruron/rod/lib/utils"
)

func main() {
	p, err := launcher.NewBrowser().Get()
	utils.E(err)

	fmt.Println(p)
}
