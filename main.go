package main

import (
    "bytes"
    "fmt"
    "io"
    "net/http"
    "os"
    "os/exec"
    "strconv"
    "strings"
    "sync"
    "time"
)

var keepAac bool
var NotLive = fmt.Errorf("Unable to extract playback URL (is the video a currently live stream?)")
var SegmentDoesNotExist = fmt.Errorf("Segment does not exit (404)")

func extractPlaybackUrl(video string) (string, error) {
    cmd := exec.Command(
        "yt-dlp",
        "--extractor-args",  "youtube:include_live_dash;skip=hls",
        "--match-filters",   "is_live",
        "--break-on-reject",
        "-f",                "bestaudio[protocol=http_dash_segments]",
        "--print",           "%(fragment_base_url)s",
        "--",
        video,
    )
    var out bytes.Buffer
    cmd.Stdout = &out
    if err := cmd.Run(); err != nil {
        return "", err
    }
    url := out.String()
    url = strings.TrimSuffix(url, "\n")
    url = strings.TrimSuffix(url, "\r")
    return url, nil
}

func sendToIcecast(server string, data io.Reader) error {
    args := []string {}
    args = append(args, "-re", "-i", "-", "-vn")
    if keepAac {
        args = append(args,
            "-content_type", "audio/aac",
            "-f",            "adts",
        )
    } else {
        args = append(args,
            "-c:a",          "libopus",
            "-vbr",          "on",
            "-b:a",          "128k",
            "-content_type", "audio/ogg",
            "-f",            "opus",
        )
    }
    args = append(args, server)
    cmd := exec.Command("ffmpeg", args...)
    cmd.Stdin = data
    return cmd.Run()
}

func startSending(icecastServer string) *io.PipeWriter {
    read, write := io.Pipe()
    go func() {
        defer read.Close()
        if err := sendToIcecast(icecastServer, read); err != nil {
            fmt.Fprintf(os.Stderr, "FFmpeg failed: %v", err)
            os.Exit(1)
        }
    }()
    return write
}

type YoutubeReader struct {
    mu      sync.Mutex
    baseUrl string
    notLive bool
    segment uint64
    video   string
}

func (r *YoutubeReader) refresh() error {
    url, err := extractPlaybackUrl(r.video)
    if err != nil {
        return err
    }
    var segment uint64
    if url != "" {
        resp, err := http.Get(url)
        if err != nil {
            return err
        }
        segment, err = strconv.ParseUint(resp.Header.Get("X-Head-Seqnum"), 10, 64)
        if err != nil {
            return fmt.Errorf("Unable to parse X-Head-Seqnum: %w", err)
        }
        if segment > 2 {
            segment -= 2
        }
        fmt.Printf("Refreshed URL, current segment=%d\n", segment)
    }

    r.mu.Lock()
    defer r.mu.Unlock()

    if url == "" {
        r.notLive = true
        return NotLive
    }

    r.baseUrl = url
    r.segment = segment
    return nil
}

func (r *YoutubeReader) refreshRetry() error {
    var last error
    for i := 0; i < 5; i++ {
        last = r.refresh()
        if last == nil || last == NotLive {
            return last
        }
        fmt.Fprintf(os.Stderr, "Refresh failed (try %d/5): %v\n", i, last)
        time.Sleep(time.Duration(i * 5) * time.Second)
    }
    return last
}

func (r *YoutubeReader) startRefreshing() {
    fmt.Println("Refreshing playback URL every 3 hours")
    for {
        time.Sleep(3 * time.Hour)
        if err := r.refreshRetry(); err != nil {
            if err == NotLive {
                fmt.Fprintf(os.Stderr, "Stream not live anymore\n")
            } else {
                fmt.Fprintf(os.Stderr, "Failed to refresh URL: %v\n", err)
            }
            break
        }
        fmt.Println("Successfully refreshed playback URL")
    }
}

func (r *YoutubeReader) readSegment(url string, out io.Writer) error {
    var last error
    for i := 0; i < 15; i++ {
        ok := func() bool {
            resp, err := http.Get(url)
            if err != nil {
                last = err
                return false
            }
            defer resp.Body.Close()
            if resp.StatusCode == 404 {
                last = SegmentDoesNotExist
                return false
            }
            if resp.StatusCode != 200 {
                last = fmt.Errorf("Non-200 response code %d", resp.StatusCode)
                return false
            }
            _, _ = io.Copy(out, resp.Body)
            return true
        }()
        if ok {
            return nil
        }
        time.Sleep(time.Second)
    }
    return last
}

func (r *YoutubeReader) run(out io.WriteCloser) error {
    defer out.Close()
    if err := r.refreshRetry(); err != nil {
        return err
    }
    go r.startRefreshing()
    for {
        url := func() string {
            r.mu.Lock()
            defer r.mu.Unlock()
            if strings.HasSuffix(r.baseUrl, "/") {
                return fmt.Sprintf("%ssq/%d", r.baseUrl, r.segment)
            } else {
                return fmt.Sprintf("%s/sq/%d", r.baseUrl, r.segment)
            }
        }()
        err := r.readSegment(url, out)
        if err == nil {
            func() {
                r.mu.Lock()
                defer r.mu.Unlock()
                r.segment++
            }()
            continue
        }
        if err == SegmentDoesNotExist {
            fmt.Fprintf(os.Stderr, "Could not find segment in time, forcing refresh\n")
        } else {
            fmt.Fprintf(os.Stderr, "Could not download segment, forcing refresh: %v\n", err)
        }
        if err = r.refreshRetry(); err != nil {
            if err == NotLive {
                break
            }
            fmt.Fprintf(os.Stderr, "Refresh failed: %v\n", err)
        }
    }
    return nil
}

func main() {
    video, icecastServer := parseArgs()

    sink := startSending(icecastServer)

    r := &YoutubeReader {
        video: video,
    }
    if err := r.run(sink); err != nil {
        fmt.Fprintf(os.Stderr, "%v", err)
        os.Exit(1)
    }
}

