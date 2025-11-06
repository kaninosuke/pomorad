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
type FileInfo struct {
	path string
	tags tag.Metadata
}

const (
	TypeUnknown MediaType = iota
	TypeFlac
	// TODO
	// Mp3
	// Aac
)
const me = "pomorad"

var cmdlineTimer uint
var cmdlineDir string

// var cmdlineTagArtist string
// var filterArtist string

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
		var playbackSecond uint
		if cmdlineTimer > 0 {
			playbackSecond = cmdlineTimer
		} else {
			playbackSecond, err = cfg.Section("pomodoro").Key(playbackTimer).Uint()
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
		// TODO
		// if cmdlineTagArtist != "" {
		// 	filterArtist = cmdlineTagArtist
		// } else {
		// 	filterArtist = cfg.Section("tag_filter").Key("artist").String()
		// }
		// if len(filterArtist) > 0 {
		// 	isTagFilterd = true
		// 	message("[config] artist : %q", filterArtist)
		// }
		message("[info] to stop playing, press 's', return")

		// recursive file search
		// var files []string
		var fileInfos []FileInfo
		err = filepath.WalkDir(dirPath, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				fmt.Printf("error reading at path %q: %v", path, err)
				return err
			}
			lowerPath := strings.ToLower(path)
			isPlayable, fileInfo, err := getFileInfo(lowerPath, d)
			if err != nil {
				fmt.Printf("error reading at path %q: %v", path, err)
				return err
			}
			if isPlayable {
				fileInfos = append(fileInfos, fileInfo)
			}
			return nil
		})
		if err != nil {
			fmt.Println("error reading files recursively: %w", err)
		}
		if len(fileInfos) == 0 {
			fmt.Println("error no file found: %w", err)
			return
		}

		var speakerInitialized bool
		// context to stop with timer.
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(playbackSecond)*time.Second)
		defer cancel()
		// goroutine to stop by user.
		go func() {
			var input string
			for {
				fmt.Scanln(&input)
				if strings.ToLower(input) == "s" {
					message("[info] stopped by user")
					cancel()
					return
				}
			}
		}()
		// Loop until the context's timeout is reached.
		for ctx.Err() == nil {
			selectedFileInfo, err := selectRandomFile(fileInfos)
			if err != nil {
				fmt.Printf("error selecting file: %v\n", err)
				continue
			}

			f, selectedFile, err := openFile(selectedFileInfo.path)
			if err != nil {
				fmt.Printf("error opening file: %v\n", err)
				continue
			}
			var tagInfo []string
			artist := selectedFileInfo.tags.Artist()
			if len(artist) != 0 {
				tagInfo = append(tagInfo, artist)
			}
			title := selectedFileInfo.tags.Title()
			if len(title) != 0 {
				tagInfo = append(tagInfo, fmt.Sprintf("%q", title))
			}
			trackInfo := strings.Join(tagInfo, " ")

			message("[♪] %s file: %q", trackInfo, selectedFile)
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

	rootCmd.Flags().UintVarP(&cmdlineTimer, "timer", "t", 0, "playback_timer")
	rootCmd.Flags().StringVarP(&cmdlineDir, "dir", "d", "", "music_dir")
	// TODO
	// rootCmd.Flags().StringVarP(&cmdlineTagArtist, "artist", "a", "", "tag:artist")
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

func getFileInfo(lowerPath string, d fs.DirEntry) (bool, FileInfo, error) {
	// pre-filter
	if !filterPlayable(lowerPath, d) {
		return false, FileInfo{}, nil
	}
	// open and inspect
	f, _, err := openFile(strings.ToLower(lowerPath))
	if err != nil {
		return false, FileInfo{}, fmt.Errorf("error opening file for tag reading: %w", err)
	}
	defer f.Close()

	m, err := tag.ReadFrom(f)
	if err != nil {
		// no tag. when tag filtered, skip file
		return true, FileInfo{lowerPath, nil}, nil
	}

	// return with media tag
	return true, FileInfo{lowerPath, m}, nil
}
func filterPlayable(lowerPath string, d fs.DirEntry) bool {
	isFile := !d.Type().IsDir()
	mediaType := TypeUnknown
	if strings.HasSuffix(lowerPath, ".flac") {
		mediaType = TypeFlac
	}
	isPlayableMediaType := !(mediaType == TypeUnknown)
	return isFile && isPlayableMediaType
}

// select randomly
func selectRandomFile(fileInfos []FileInfo) (FileInfo, error) {
	if len(fileInfos) == 0 {
		return FileInfo{}, fmt.Errorf("no files to select from")
	}
	randomIndex := rand.IntN(len(fileInfos))
	return fileInfos[randomIndex], nil
}

// open file
func openFile(selected string) (*os.File, string, error) {
	f, err := os.Open(selected)
	if err != nil {
		return nil, selected, fmt.Errorf("error opening file %s: %w", selected, err)
	}
	return f, selected, nil
}
