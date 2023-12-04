package main

import (
	"fmt"
	"github.com/spf13/pflag"
)

func main() {
	var x = "default"
	pflag.StringVar(&x, "x", x, "X=")
	pflag.Parse()

	fmt.Println(x)

}
