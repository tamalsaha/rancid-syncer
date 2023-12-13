package main

import (
	"kmodules.xyz/client-go/tools/parser"
	_ "kmodules.xyz/client-go/tools/parser"
	"os"
)

func main() {
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
