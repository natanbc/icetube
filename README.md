# Icetube

Pipes audio from a youtube livestream into an icecast server.

Requires `yt-dlp` and `ffmpeg` to be present in PATH.

# Usage

```
Usage: icetube [OPTIONS] <youtube video link/id> <icecast server URL>

Options:
    --keep-aac
        Don't convert the youtube audio to opus. AAC might work with icecast,
        but is not supported.

Examples:
    icetube https://youtu.be/jfKfPfyJRdk icecast://source:PASSWORD@my-server:8001/stream.ogg
```

