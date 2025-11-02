package cmd

import (
	"context"
	"fmt"
	"io/fs"
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

type MediaType int

const (
	Unknown MediaType = iota
	Flac
	// TODO
	// Mp3
	// Aac
)
const me = "pomorad"

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
			return
		}
		playingSecond, err := cfg.Section("pomodoro").Key("playing_second").Int()
		if err != nil {
			fmt.Printf("Failed to parse 'playing_second' as an integer from config: %v\n", err)
			return
		}
		message("playing_second: %d", playingSecond)
		dirPath := cfg.Section("path").Key("music_dir").String()
		if dirPath == "" {
			fmt.Println("'music_dir' not found in [path] section of config.ini")
			return
		}

		// recursive file search
		var files []string
		err = filepath.WalkDir(dirPath, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				fmt.Printf("error reading at path %q: %v", path, err)
				return err
			}
			_, isPlayable := resolveMediaType(path)
			if !d.IsDir() && isPlayable {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			fmt.Println("error reading files recursively: %w", err)
		}
		if len(files) == 0 {
			fmt.Println("error no file found: %w", err)
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(playingSecond)*time.Second)
		defer cancel()

	Loop:
		for {
			select {
			case <-ctx.Done():
				fmt.Println("\nPlayback finished.")
				break Loop
			default:
				f, selectedFile, err := selectAndOpenFile(files)
				if err != nil {
					fmt.Printf("error opening file: %v\n", err)
					continue
				}

				err = playFlac(ctx, f)
				f.Close()
				if err != nil {
					fmt.Printf("error playing file %s: %v\n", selectedFile, err)
				}
			}
		}
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

func message(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	fmt.Printf("[%s] %s\n", me, msg)
}

func playFlac(ctx context.Context, f *os.File) error {
	streamer, format, err := flac.Decode(f)
	if err != nil {
		return fmt.Errorf("failed to decode media: %w", err)
	}
	defer streamer.Close() // streamer must be closed to release resources
	speaker.Init(format.SampleRate, format.SampleRate.N(time.Second/10))
	done := make(chan bool)
	speaker.Play(beep.Seq(streamer, beep.Callback(func() {
		done <- true
	})))

	select {
	case <-done:
		// Playback finished successfully
	case <-ctx.Done():
		speaker.Clear()
		fmt.Println("\nPlayback interrupted by timer.")
	}
	return nil
}
func resolveMediaType(path string) (MediaType, bool) {
	lowerPath := strings.ToLower(path)
	// TODO can play flac only
	if strings.HasSuffix(lowerPath, ".flac") {
		return Flac, true
	}
	return Unknown, false
}

func selectAndOpenFile(files []string) (*os.File, string, error) {
	if len(files) == 0 {
		return nil, "", fmt.Errorf("no files to select from")
	}
	randomIndex := rand.IntN(len(files))
	selected := files[randomIndex]
	fmt.Println(selected)

	f, err := os.Open(selected)
	if err != nil {
		return nil, selected, fmt.Errorf("error opening file %s: %w", selected, err)
	}
	return f, selected, nil
}
