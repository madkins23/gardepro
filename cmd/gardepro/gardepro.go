/*
GardePro renames and moves jpg and mp4 files from GardePro deer cameras.
The files are renamed with the images and videos are taken and
copied to a specified repository.

The naming convention is:
    Year/Mon-Day-Hour:Minute:Second-BaseName.Ext
where
    * Year is a subdirectory under the target root directory (created if required)
    * Month, day, and time are taken from the media file properties (not the source directory)
    * BaseName.Ext is the source file basename and extension

This application was written for a fairly narrow set of personal requirements and
assumptions instead of as a more general application that may serve other needs.
Please feel free to copy and modify the code for your own needs.

Usage:

    gardepro [flags]

The flags are:

    -source
        Source file path (required).
    -target
        Target root directory (required)
    -console
        Log to the console instead of the specified log file [false]
    -log
        Log file path [/tmp/gardepro.log]
*/
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/abema/go-mp4"
	"github.com/dsoprea/go-exif/v3"
	exifcommon "github.com/dsoprea/go-exif/v3/common"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/sqweek/dialog"
	"github.com/udhos/equalfile"
)

var (
	fileCompare   = equalfile.New(nil, equalfile.Options{})
	flags         *flag.FlagSet
	localTimeZone = time.Now().Location()
)

func main() {
	var console bool
	var logFile, source, target string

	flags = flag.NewFlagSet("gardepro", flag.ContinueOnError)
	flags.BoolVar(&console, "console", false, "Direct log to console")
	flags.StringVar(&logFile, "log", "/tmp/gardepro.log", "Path to log file")
	flags.StringVar(&source, "source", "", "Source image directory to be fixed")
	flags.StringVar(&target, "target", "", "Target directory for image files")
	if err := flags.Parse(os.Args[1:]); err != nil {
		dialog.Message(err.Error()).Title("Error parsing command line flags").Error()
		return
	}

	if source == "" || target == "" {
		dialog.Message("Missing command line flag -source or -target").Title("Error parsing command line flags").Error()
		return
	}

	zerolog.TimestampFunc = func() time.Time {
		return time.Now().Local()
	}
	if console {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: "15:04:05"})
	} else if f, err := os.OpenFile(logFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666); err != nil {
		dialog.Message(err.Error()).Title("Log File Creation").Error()
		return
	} else {
		defer func() { _ = f.Close() }()
		_, _ = fmt.Fprintln(f) // Separate blocks of log statements.
		// Use ConsoleWriter for readable text instead of JSON blocks.
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: f, TimeFormat: "15:04:05", NoColor: true})
	}

	target = strings.TrimSuffix(target, "/")

	log.Logger = log.Logger.With().Str("source", source).Logger()
	log.Logger = log.Logger.With().Str("target", target).Logger()

	log.Info().Msg("GardePro starting")
	defer log.Info().Msg("GardePro finished")

	var targetDir string
	var targetPath string

	const (
		fileDateStubFmt = "/2006/01-02-15:04:05-"
		targetDirFmt    = "/2006"
		tagIDDateTime   = 0x132
		tagNameDateTime = "Date Time"
	)

	switch ext := strings.ToLower(filepath.Ext(source)); ext {
	case ".jpg", ".jpeg":
		if index, err := EXIFgetIndex(source); err != nil {
			errorFatal("Get EXIF index", err, nil)
		} else if whenValue, err := EXIFgetValue(index, tagNameDateTime, tagIDDateTime); err != nil {
			errorFatal("Get tag value", err, func(event *zerolog.Event) *zerolog.Event {
				return event.Str("tag", tagNameDateTime).
					Str("ID", "0x"+strconv.FormatUint(uint64(tagIDDateTime), 16))
			})
		} else if whenStr, ok := whenValue.(string); !ok {
			errorFatal("Date/Time not string", err, func(event *zerolog.Event) *zerolog.Event {
				return event.Interface("value", whenValue)
			})
		} else if when, err := time.Parse("2006:01:02 15:04:05", whenStr); err != nil {
			errorFatal("Parse time", err, func(event *zerolog.Event) *zerolog.Event {
				return event.Str("when", whenStr)
			})
		} else {
			// Parsed as UTC (even though it was local time) since no time zone in string.
			// Go ahead format it as UTC, it will look like it was local all along.
			targetDir = target + when.Format(targetDirFmt)
			targetPath = target + when.Format(fileDateStubFmt) + filepath.Base(source)
		}
	case ".mp4":
		if metadata, err := MP4getMetadata(source); err != nil {
			errorFatal("Get MP4 metadata", err, nil)
		} else if len(metadata) != 1 {
			errorFatal("Wrong number of metadata results", nil, func(event *zerolog.Event) *zerolog.Event {
				return event.Int("number", len(metadata))
			})
		} else if payload, ok := metadata[0].Payload.(*mp4.Mvhd); !ok {
			errorFatal("Convert metadata payload to mvhd", nil, func(event *zerolog.Event) *zerolog.Event {
				return event.Interface("payload", metadata[0].Payload)
			})
		} else {
			// Mvhd/CreationTimeV0 is seconds since Jan 1, 1904 for some reason.
			when := time.Date(1904, time.January, 1, 0, 0, 0, 0, time.UTC).
				Add(time.Second * time.Duration(payload.CreationTimeV0)).
				// It's also in UTC so convert it to the local time zone.
				In(localTimeZone)
			targetDir = target + when.Format(targetDirFmt)
			targetPath = target + when.Format(fileDateStubFmt) + filepath.Base(source)
		}
	default:
		errorFatal("Unrecognized extension: "+ext, nil, nil)
	}

	if targetDir == "" {
		errorFatal("No target dir", nil, nil)
	} else if targetPath == "" {
		errorFatal("No target path", nil, nil)
	}

	extraTargetFn := func(event *zerolog.Event) *zerolog.Event {
		return event.Str("target-path", targetPath).Str("target-dir", targetDir)
	}
	if err := checkTargetDir(targetDir); err != nil {
		errorFatal("Check target dir", err, extraTargetFn)
	}
	if err := copySourceToTarget(source, targetPath, extraTargetFn); err != nil {
		errorFatal("Copy source file to target directory", err, extraTargetFn)
	}
}

func checkTargetDir(targetDir string) error {
	if stat, err := os.Stat(targetDir); err == nil {
		if !stat.IsDir() {
			return fmt.Errorf("target dir is not a directory")
		}
	} else if errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(targetDir, 0766); err != nil {
			return fmt.Errorf("make target dir: %w", err)
		}
	} else {
		return fmt.Errorf("stat target dir: %w", err)
	}
	return nil
}

func copySourceToTarget(source, target string, extra func(*zerolog.Event) *zerolog.Event) error {
	if _, err := os.Stat(target); err == nil {
		if equal, err := fileCompare.CompareFile(source, target); err != nil {
			return fmt.Errorf("compare files: %w", err)
		} else if equal {
			extra(log.Info()).Msg("Skipping pre-existing identical file")
		} else {
			return fmt.Errorf("pre-existing file not identical")
		}
	} else if errors.Is(err, os.ErrNotExist) {
		if err := copyFile(source, target); err != nil {
			return fmt.Errorf("copy file: %w", err)
		} else {
			extra(log.Info()).Msg("Copied file")
		}
	} else {
		return fmt.Errorf("stat target file: %w", err)
	}
	return nil
}

func copyFile(source, target string) error {
	sourceFile, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open source file: %w", err)
	}
	defer func() { _ = sourceFile.Close() }()
	targetFile, err := os.Create(target)
	if err != nil {
		return fmt.Errorf("create target file: %w", err)
	}
	defer func() { _ = targetFile.Close() }()
	if _, err = io.Copy(targetFile, sourceFile); err != nil {
		return fmt.Errorf("copy file: %w", err)
	}
	return nil
}

func errorFatal(message string, err error, extra func(*zerolog.Event) *zerolog.Event) {
	msg := message
	if err != nil {
		msg += ":\n" + err.Error()
	}
	dialog.Message(msg).Title("Fatal Error").Error()
	// Fatal() will call os.Exit() after logging, skipping defer statements in main().
	event := log.Fatal()
	if err != nil {
		event = event.Err(err)
	}
	if extra != nil {
		event = extra(event)
	}
	event.Msg(message)
}

func EXIFenumerateIndex(index exif.IfdIndex) error {
	err := index.RootIfd.EnumerateTagsRecursively(func(ifd *exif.Ifd, ite *exif.IfdTagEntry) error {
		log.Debug().Str("path", ite.IfdPath()+"/"+ite.TagName()).
			Str("ID", "0x"+strconv.FormatUint(uint64(ite.TagId()), 16)).Msg("tag")
		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

func EXIFgetIndex(path string) (exif.IfdIndex, error) {
	var index exif.IfdIndex
	if rawExif, err := exif.SearchFileAndExtractExif(path); err != nil {
		return index, fmt.Errorf("getting EXIF from file: %w", err)
	} else if im, err := exifcommon.NewIfdMappingWithStandard(); err != nil {
		return index, fmt.Errorf("getting EXIF mapping: %w", err)
	} else {
		ti := exif.NewTagIndex()
		if _, index, err = exif.Collect(im, ti, rawExif); err != nil {
			return index, fmt.Errorf("getting EXIF index: %w", err)
		} else {
			return index, nil
		}
	}
}

func EXIFgetValue(index exif.IfdIndex, tagName string, tagID uint16) (interface{}, error) {
	tagResults, err := index.RootIfd.FindTagWithId(tagID)
	if err != nil {
		tagResults, err = index.Lookup["IFD/Exif"].FindTagWithId(tagID)
	}
	if err != nil {
		log.Error().Err(err).Str("tag", tagName).Uint16("ID", tagID).
			Msg("Find EXIF tag by ID")
		if err2 := EXIFenumerateIndex(index); err2 != nil {
			log.Error().Err(err2).Msg("Enumerating EXIF index")
		}
		return "", fmt.Errorf("find EXIF tag: %w", err)
	}
	if len(tagResults) != 1 {
		return "", fmt.Errorf("wrong number of EXIF tag results: %d", len(tagResults))
	} else if value, err := tagResults[0].Value(); err != nil {
		return "", fmt.Errorf("getting EXIF tag value: %w", err)
	} else {
		return value, nil
	}
}

func MP4getMetadata(path string) ([]*mp4.BoxInfoWithPayload, error) {
	if file, err := os.Open(path); err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	} else {
		return mp4.ExtractBoxWithPayload(file, nil,
			mp4.BoxPath{mp4.BoxTypeMoov(), mp4.BoxTypeMvhd()})
	}
}
