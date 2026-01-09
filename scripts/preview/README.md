# UI Demo GIF Generator

This script captures an animated GIF/WebP showcasing the ooo storage explorer UI.

## Prerequisites

- Node.js
- ffmpeg (for video conversion)
- Puppeteer dependencies

## Setup

```bash
cd scripts/preview
npm install
```

## Usage

1. Start the demo server:
```bash
go run demo_server.go
```

2. In another terminal, run the capture script:
```bash
node capture.js
```

The script will:
- Navigate through the storage explorer UI
- Perform push and edit operations with human-like typing
- Capture frames and convert them to animated GIF/WebP
- Output files to `public/ui-demo.gif` and `public/ui-demo.webp`
