# GardePro

GardePro renames and moves JPG and MP4 files from GardePro deer cameras.

## Background

I have two GardePro deer cameras looking across the pasture behind my house.
These generate JPG and MP4 files in a fairly standard format
within the DCIM directory of an inserted memory card.

Periodically I bring in the memory cards and look through the images and videos.
I have been copying interesting ones to my NAS.
The interesting ones are sparse (there are a lot of duds).
Over time I realized that some of them had the same file names and
were overwriting the previous files.

This application renames an individual file and copies it to the NAS.
The renaming is done in a way that:

* separates the media into subdirectories by year,
* begins with the date and time so name ordering is chronological, and
* preserves the original basename (for no good reason).

I am counting on the two cameras not taking useful pictures
at the exact same second with the exact same base name.
There is code to check for non-identical overwrites.

## Installation and Usage

This isn't the command-line usage which can be found in the
[application source](https://github.com/madkins23/gardepro/blob/main/cmd/gardepro/gardepro.go),
the [godoc](https://pkg.go.dev/github.com/madkins23/gardepro/cmd/gardepro),
or by building and running it without arguments.
This section describes how I configure the application on my system.

When I thought about how I wanted to use this application,
I decided that the simplest thing would be to drag and drop
a file onto a desktop icon.
I created the application to work on a single source file at a time
and hooked it into a `.desktop` file within the `~/Desktop` directory.
This is my `~/Desktop/coyotes.desktop` file:

    [Desktop Entry]
    Version=0.1
    Name=Coyotes
    Comment=Target for dropping photos of coyotes to go to NAS
    Exec=/home/me/bin/gardepro -source=%f -target=/home/me/photos/Homes/Canterwood/Wildlife/Coyotes
    Terminal=false
    Type=Application
    Categories=Utility;Application;

The file shows up on the desktop as a generic icon
since I didn't bother to configure a custom icon.
When I drag and drop a file from the memory chip it passes the
file's path into the `%f` argument in the `Exec` string.
The application runs once for each such file,
renaming and moving the file as I desire.

I'm surprised and pleased at how it easy it was to get the drag and drop behavior.
The application may be weird hacky crap but this bit is really cool. ;-)

## Modules

I use the following Go modules:

* [github.com/abema/go-mp4](github.com/abema/go-mp4) to get MP4 creation date/time
* [github.com/dsoprea/go-exif](github.com/dsoprea/go-exif) to get JPG creation date/time
* [github.com/rs/zerolog](github.com/rs/zerolog) for pretty logging
* [github.com/sqweek/dialog](github.com/sqweek/dialog)
  to display error messages directly to the user as they occur
  (they are also logged to a file)
* [github.com/udhos/equalfile](github.com/udhos/equalfile) to compare files
  in the case of duplicate target paths
