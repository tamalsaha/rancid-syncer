package main

import (
	"fmt"
	"kmodules.xyz/client-go/tools/parser"
	_ "kmodules.xyz/client-go/tools/parser"
	"os"
	"strings"
)

func main() {
	ref := "v0.0.1)_8.0.35-oracle"
	out := encodeTag(ref, "v0.2.0")
	fmt.Println(out)

	o2 := decodeTag(out)
	fmt.Println(o2)
}

func encodeTag(ref, tag string) string {
	var sb strings.Builder
	replace := false
	for _, ch := range ref {
		if ch == '(' {
			replace = true
			sb.WriteRune('(')
			sb.WriteString(tag)
			sb.WriteRune(')')
		} else if ch == ')' {
			replace = false
		} else if !replace {
			sb.WriteRune(ch)
		}
	}
	return sb.String()
}

func decodeTag(ref string) string {
	r := strings.NewReplacer("(", "", ")", "")
	return r.Replace(ref)
}

func main_() {
	fsys := os.DirFS("/Users/tamal/go/src/kubedb.dev/provider-aws/examples")
	_, err := parser.ListFSResources(fsys)
	if err != nil {
		panic(err)
	}
	//for _, ri := range resources {
	//	fmt.Println(ri.Filename)
	//}

	//err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
	//	if err != nil {
	//		return err
	//	}
	//	parser.ListPathResources()
	//
	//	return nil
	//})
	if err != nil {
		panic(err)
	}
}
