package main

import (
	"fmt"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"strings"
)

func main() {
	str := "charts.x-helm.dev-v1alpha1-clusterchartpresets"
	gvr, err := ParseGVR(str)
	if err != nil {
		panic(err)
	}
	fmt.Printf("%+v\n", *gvr)
}

func ParseGVR(name string) (*schema.GroupVersionResource, error) {
	name = reverse(name)
	parts := strings.SplitN(name, "-", 3)
	if len(parts) != 3 {
		return nil, fmt.Errorf("%s is not a valid gvr encoded name", name)
	}
	gvr := schema.GroupVersionResource{
		Group:    reverse(parts[2]),
		Version:  reverse(parts[1]),
		Resource: reverse(parts[0]),
	}
	if gvr.Group == "core" {
		gvr.Group = ""
	}
	return &gvr, nil
}

// ref: https://groups.google.com/g/golang-nuts/c/oPuBaYJ17t4/m/PCmhdAyrNVkJ
func reverse(input string) string {
	// Get Unicode code points.
	n := 0
	rune := make([]int32, len(input))
	for _, r := range input {
		rune[n] = r
		n++
	}
	rune = rune[0:n]

	// Reverse
	for i := 0; i < n/2; i++ {
		rune[i], rune[n-1-i] = rune[n-1-i], rune[i]
	}

	// Convert back to UTF-8.
	return string(rune)
}
