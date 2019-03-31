# MP3 Metafix

This tool helps you rename and update those dumb MP3's you downloaded from the 
internet. It performs a search through Spotify's Web API to find the relative
metadata information and write to it.

Usage: `metafix [flags] filename` use `-h` for more info or read the source.
The filename can be a single file or a directory in which it will iterate over
MP3 files.

I use build tags to assign the Spotify credentials. Please look into Golang
build flags as well as how to get your own Spotify credentials.