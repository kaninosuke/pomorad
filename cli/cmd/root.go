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

	"github.com/dhowden/tag"
	"github.com/faiface/beep"
	"github.com/faiface/beep/flac"
	"github.com/faiface/beep/speaker"
	"github.com/spf13/cobra"
	"gopkg.in/ini.v1"
)

type MediaType int

const (
	TypeUnknown MediaType = iota
	TypeFlac
	// TODO
	// Mp3
	// Aac
)
const me = "pomorad"

var cmdlineTimer int
var cmdlineDir string
var cmdlineTagArtist string

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
		message("[info] started at %s", time.Now().Format(time.RFC3339))

		cfg, err := ini.Load("config.ini")
		if err != nil {
			fmt.Printf("Failed to read config file: %v\n", err)
			return
		}
		playbackTimer := "playback_timer"
		var playbackSecond int
		if cmdlineTimer > 0 {
			playbackSecond = cmdlineTimer
		} else {
			playbackSecond, err = cfg.Section("pomodoro").Key(playbackTimer).Int()
			if err != nil {
				fmt.Printf("Failed to parse '%s' as an integer from config: %v\n", playbackTimer, err)
				return
			}
		}
		message("[config] %s : %d", playbackTimer, playbackSecond)
		var dirPath string
		if cmdlineDir != "" {
			dirPath = cmdlineDir
		} else {
			dirPath = cfg.Section("path").Key("music_dir").String()
			if dirPath == "" {
				fmt.Println("'music_dir' not found in [path] section of config.ini")
				return
			}
		}
		message("[config] music_dir : %q", dirPath)

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

		var speakerInitialized bool
		// context to stop with timer.
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(playbackSecond)*time.Second)
		defer cancel()
		// goroutine to stop by user.
		go func() {
			message("[info] to stop playing, press 's' & return")
			var input string
			for {
				fmt.Scanln(&input)
				if input == "s" || input == "S" {
					message("[info] stopped by user")
					cancel()
					return
				}
			}
		}()
		// Loop until the context's timeout is reached.
		for ctx.Err() == nil {
			selectedFilePath, err := selectFile(files)
			if err != nil {
				fmt.Printf("error selecting file: %v\n", err)
				continue
			}

			trackInfo, err := readTrackInfo(selectedFilePath)
			if err != nil {
				fmt.Printf("error reading track info: %v\n", err)
				continue
			}
			f, selectedFile, err := openFile(selectedFilePath)
			if err != nil {
				fmt.Printf("error opening file: %v\n", err)
				continue
			}

			message("[♪] %s file:%q", trackInfo, selectedFile)
			err = playTrack(ctx, f, &speakerInitialized)
			f.Close() // Ensure file is closed after playing
			if err != nil {
				fmt.Printf("error playing file %s: %v\n", selectedFile, err)
			}
		}
		message("done at %s", time.Now().Format(time.RFC3339))
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
	// rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")

	rootCmd.Flags().IntVarP(&cmdlineTimer, "timer", "t", 0, "playback_timer")
	rootCmd.Flags().StringVarP(&cmdlineDir, "dir", "d", "", "music_dir")
	rootCmd.Flags().StringVarP(&cmdlineTagArtist, "artist", "a", "", "tag:artist")
}

// custom print()
func message(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	fmt.Printf("[%s♪] %s\n", me, msg)
}

// controls player
func playTrack(ctx context.Context, f *os.File, speakerInitialized *bool) error {
	streamer, format, err := flac.Decode(f)
	if err != nil {
		return fmt.Errorf("failed to decode media: %w", err)
	}
	defer streamer.Close()

	if !*speakerInitialized {
		speaker.Init(format.SampleRate, format.SampleRate.N(time.Second/10))
		*speakerInitialized = true
	}
	done := make(chan bool)
	speaker.Play(beep.Seq(streamer, beep.Callback(func() {
		done <- true
	})))

	select {
	case <-done:
		// Playback finished successfully
	case <-ctx.Done():
		speaker.Clear()
	}
	return nil
}

// resolve type for beep.
func resolveMediaType(path string) (MediaType, bool) {
	lowerPath := strings.ToLower(path)
	// TODO can play flac only
	if strings.HasSuffix(lowerPath, ".flac") {
		return TypeFlac, true
	}
	tagfliter(lowerPath)
	return TypeUnknown, false
}

func tagfliter(lowerPath string) {
	// TODO co-use with readTrackInfo?
}

// select randomly
func selectFile(files []string) (string, error) {
	if len(files) == 0 {
		return "", fmt.Errorf("no files to select from")
	}
	randomIndex := rand.IntN(len(files))
	selected := files[randomIndex]
	return selected, nil
}

// open file
func openFile(selected string) (*os.File, string, error) {
	f, err := os.Open(selected)
	if err != nil {
		return nil, selected, fmt.Errorf("error opening file %s: %w", selected, err)
	}
	return f, selected, nil
}

// read tag
func readTrackInfo(filePath string) (string, error) {
	f, _, err := openFile(filePath)
	if err != nil {
		return "", fmt.Errorf("error opening file for tag reading: %w", err)
	}
	defer f.Close()

	m, err := tag.ReadFrom(f)
	if err != nil {
		return "", nil
	}

	var trackInfoParts []string
	artist := m.Artist()
	if len(artist) > 0 {
		trackInfoParts = append(trackInfoParts, artist)
	}
	title := m.Title()
	if len(title) > 0 {
		trackInfoParts = append(trackInfoParts, fmt.Sprintf("\"%s\"", title))
	}
	return strings.Join(trackInfoParts, " "), nil
}
