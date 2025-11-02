/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"io/fs"
	"log"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/faiface/beep"
	"github.com/faiface/beep/flac"
	"github.com/faiface/beep/speaker"
	"github.com/spf13/cobra"
	"gopkg.in/ini.v1"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "pomorad",
	Short: "A brief description of your application",
	Long: `A longer description that spans multiple lines and likely contains
examples and usage of using your application. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := ini.Load("config.ini")
		if err != nil {
			fmt.Printf("Failed to read config file: %v\n", err)
			fmt.Println("Please create a 'config.ini' file with '[path]' section and 'music_dir' key.")
			return
		}

		dirPath := cfg.Section("path").Key("music_dir").String()
		if dirPath == "" {
			fmt.Println("'music_dir' not found in [path] section of config.ini")
			return
		}
		var files []string
		err = filepath.WalkDir(dirPath, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				fmt.Printf("err reading at path %q: %v", path, err)
			}
			// TODO can play flac only
			if !d.IsDir() && strings.HasSuffix(strings.ToLower(path), "flac") {
				files = append(files, path)
			}

			return nil
		})
		if err != nil {
			fmt.Println("err reading files recursively: %w", err)
		}

		if len(files) == 0 {
			fmt.Println("err no file found: %w", err)
			return
		}
		randomIndex := rand.IntN(len(files))
		selected := files[randomIndex]
		fmt.Println(selected)
		f, err := os.Open(selected)
		if err != nil {
			fmt.Println("err opening file: %w%w", f, err)
		}
		defer f.Close()
		// TODO can play flac only
		playFlac(f)
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.pomorad.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

func playFlac(f *os.File) {
	// playing flac
	streamer, format, err := flac.Decode(f)
	if err != nil {
		log.Fatal(err)
	}
	defer streamer.Close()
	speaker.Init(format.SampleRate, format.SampleRate.N(time.Second/10))
	done := make(chan bool)
	speaker.Play(beep.Seq(streamer, beep.Callback(func() {
		done <- true
	})))

	<-done
}
