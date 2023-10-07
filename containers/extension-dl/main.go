package main

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/hashicorp/go-getter/v2"
)

const defaultDownloadDir string = "/extensions"

func main() {
	var dst string
	flag.StringVar(&dst, "dst", defaultDownloadDir, "Target download directory.")
	flag.Parse()

	extensions := flag.Args()
	if len(extensions) < 1 {
		log.Print("No extensions to download, nothing to do!")
		os.Exit(0)
	}

	for _, src := range extensions {
		if err := download(context.Background(), src, dst); err != nil {
			log.Fatal(err)
		}
	}

}

func download(ctx context.Context, src, dst string) error {
	pwd, err := os.Getwd()
	if err != nil {
		return err
	}

	req := &getter.Request{
		Src:     src,
		Dst:     dst,
		Pwd:     pwd,
		GetMode: getter.ModeAny,
	}

	_, err = getter.DefaultClient.Get(ctx, req)
	if err != nil {
		return err
	}

	return nil
}
