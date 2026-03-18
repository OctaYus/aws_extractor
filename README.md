# AWS S3 Bucket Extractor

A high-performance concurrent web crawler written in Go that extracts AWS S3 bucket references from websites. Perfect for security research, bug bounty hunting, and cloud asset discovery.

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://golang.org/doc/install)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

## Features

- **Fast Concurrent Crawling** - Multi-threaded URL processing
- **11 Regex Patterns** - Detects various S3 bucket URL formats
- **Bucket Validation** - Filters false positives automatically
- **Real-time Saving** - Buckets saved immediately when discovered
- **Multiple Output Formats** - TXT, JSON, and CSV support
- **Colored Terminal Output** - Easy-to-read colored logs
- **Retry Logic** - Automatic retry for failed requests
- **Customizable** - Configurable timeout, workers, and User-Agent

## Installation

### Option 1: Install with `go install` (Recommended)
```bash
go install github.com/OctaYus/aws_extractor@latest
```

This will install the binary to your `$GOPATH/bin` directory (usually `~/go/bin`).

Make sure `$GOPATH/bin` is in your PATH:
```bash
export PATH=$PATH:$(go env GOPATH)/bin
```

### Option 2: Build from Source
```bash
# Clone the repository
git clone https://github.com/OctaYus/aws_extractor.git
cd aws_extractor

# Build the binary
go build -o aws_extractor

# Optional: Install to your system
sudo mv aws_extractor /usr/local/bin/
```

### Option 3: Download Pre-built Binary

Download the latest release from the [Releases page](https://github.com/OctaYus/aws_extractor/releases).

## Quick Start
```bash
# Basic usage
aws_extractor -u urls.txt

# Save results to JSON
aws_extractor -u urls.txt -o results.json -f json

# Verbose mode with 10 workers
aws_extractor -u urls.txt -v -w 10

# Custom timeout and debug mode
aws_extractor -u urls.txt -t 60 -debug
```

## Usage
```
Usage of aws_extractor:
  -u string
        Path to file containing URLs (one per line) (required)
  -o string
        Output file path
  -f string
        Output format (txt, json, csv) (default "txt")
  -w int
        Number of concurrent workers (default 5)
  -t int
        Request timeout in seconds (default 30)
  -v    Verbose output (show progress per URL)
  -user-agent string
        Custom User-Agent string
  -debug
        Enable debug logging
```

## URL File Format

Create a text file with one URL per line:
```
# My target URLs
https://example.com
https://github.com
amazon.com          # Will become https://amazon.com

# Lines starting with # or // are ignored
// Another comment
google.com

# Blank lines are ignored
```

## Output Files

When you specify an output file with `-o`, two files are created:

1. **Main Results File** (`results.txt` / `results.json` / `results.csv`)
   - Complete crawl results with all URLs and their status

2. **Buckets File** (`results_buckets.txt`)
   - Real-time list of discovered buckets
   - Format: `bucket-name | source-url`
   - Updated immediately as buckets are found

## Examples

### Basic Scan
```bash
aws_extractor -u targets.txt
```

### Production Scan with JSON Output
```bash
aws_extractor -u targets.txt -o scan_results.json -f json
```

### High-Speed Scan (10 workers, 60s timeout)
```bash
aws_extractor -u targets.txt -w 10 -t 60 -v
```

### Debug Mode (See All Pattern Matches)
```bash
aws_extractor -u targets.txt -debug
```

### Custom User-Agent
```bash
aws_extractor -u targets.txt -user-agent "MyScanner/1.0"
```

## Detected S3 Bucket Formats

The tool detects buckets in various formats:

1. **Path-style URLs**:
   - `https://s3.amazonaws.com/bucket-name/file.jpg`
   - `https://s3-us-west-2.amazonaws.com/bucket-name/`

2. **Virtual-hosted-style URLs**:
   - `https://bucket-name.s3.amazonaws.com/`
   - `https://bucket-name.s3-us-west-2.amazonaws.com/`
   - `https://bucket-name.s3.us-west-2.amazonaws.com/`

3. **S3 URI**:
   - `s3://bucket-name/path/to/file`

4. **ARN format**:
   - `arn:aws:s3:::bucket-name`

5. **Configuration files** (JSON/JS):
   - `"bucket": "bucket-name"`
   - `Bucket: "bucket-name"`
   - `bucketName: "bucket-name"`

## Output Formats

### TXT Format (`-f txt`)
```
AWS S3 Bucket Crawler Results
================================================================================

URL: https://example.com
Status: 200
Buckets found (2):
  - my-bucket-name
  - assets-bucket
--------------------------------------------------------------------------------
```

### JSON Format (`-f json`)
```json
[
  {
    "url": "https://example.com",
    "status": 200,
    "buckets": ["my-bucket-name", "assets-bucket"],
    "error": ""
  }
]
```

### CSV Format (`-f csv`)
```csv
URL,Status,Buckets Found,Bucket Names,Error
https://example.com,200,2,"my-bucket-name, assets-bucket",
```

## Terminal Output

When a bucket is discovered, you'll see:
```
────────────────────────────────────────────────────────────────────────────────
[BUCKET DISCOVERED]
  Name:   my-production-bucket
  Source: https://example.com
────────────────────────────────────────────────────────────────────────────────
```

Summary at the end:
```
================================================================================
[+] CRAWL SUMMARY
[+] Total URLs crawled: 25
[+] Successful crawls: 23
[-] Failed crawls: 2
[+] Unique buckets found: 5
[+] All unique buckets:
    - bucket-one
    - bucket-two
    - bucket-three
    - bucket-four
    - bucket-five
================================================================================
```

## Performance Tips

1. **Increase Workers** for faster scanning:
```bash
   aws_extractor -u urls.txt -w 20
```

2. **Adjust Timeout** for slow sites:
```bash
   aws_extractor -u urls.txt -t 60
```

3. **Use Verbose Mode** to monitor progress:
```bash
   aws_extractor -u urls.txt -v
```

## Security & Legal

**Important**: 
- Only scan websites you have permission to scan
- Respect `robots.txt` and terms of service
- Don't access buckets that don't belong to you
- Use for security research and bug bounties only
- Be aware of legal implications in your jurisdiction

## Troubleshooting

### "command not found: aws_extractor"

Make sure `$GOPATH/bin` is in your PATH:
```bash
export PATH=$PATH:$(go env GOPATH)/bin
```

Add this to your `~/.bashrc` or `~/.zshrc` to make it permanent.

### Getting Rate Limited

Reduce the number of workers:
```bash
aws_extractor -u urls.txt -w 3
```

### Timeouts

Increase the timeout value:
```bash
aws_extractor -u urls.txt -t 60
```

### No Buckets Found

Enable debug mode to see pattern matching details:
```bash
aws_extractor -u urls.txt -debug
```

## Building for Different Platforms
```bash
# Linux
GOOS=linux GOARCH=amd64 go build -o aws_extractor-linux

# Windows
GOOS=windows GOARCH=amd64 go build -o aws_extractor.exe

# macOS (Intel)
GOOS=darwin GOARCH=amd64 go build -o aws_extractor-mac-intel

# macOS (Apple Silicon)
GOOS=darwin GOARCH=arm64 go build -o aws_extractor-mac-arm
```

## Project Structure
```
aws_extractor/
├── main.go              # Main application code
├── go.mod               # Go module dependencies
├── go.sum               # Dependency checksums
├── README.md            # This file
├── LICENSE              # License file
└── examples/
    └── urls.txt         # Example URL file
```

## Dependencies

- [logrus](https://github.com/sirupsen/logrus) - Structured logging

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -am 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## Roadmap

- Add support for custom regex patterns
- Implement rate limiting
- Add proxy support
- Check bucket accessibility
- Add GitHub Actions for releases
- Docker container support

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- Inspired by security research tools and bug bounty workflows
- Built with Go for maximum performance and concurrency

## Author

**OctaYus**

- GitHub: [@OctaYus](https://github.com/OctaYus)
- Repository: [aws_extractor](https://github.com/OctaYus/aws_extractor)

## Support

If you find this tool useful, please consider:
- Starring the repository
- Reporting bugs via [Issues](https://github.com/OctaYus/aws_extractor/issues)
- Suggesting features via [Issues](https://github.com/OctaYus/aws_extractor/issues)

---

**Happy Hunting!**