package main

import (
	"fmt"
	"time"
)

const (
	KB = 1024
	MB = 1024 * KB
	GB = 1024 * MB
)

func formatSpeed(bytesPerSec float64) string {
	switch {
	case bytesPerSec >= GB:
		return fmt.Sprintf("%.2f GB/s", bytesPerSec/GB)
	case bytesPerSec >= MB:
		return fmt.Sprintf("%.2f MB/s", bytesPerSec/MB)
	case bytesPerSec >= KB:
		return fmt.Sprintf("%.2f KB/s", bytesPerSec/KB)
	default:
		return fmt.Sprintf("%.0f B/s", bytesPerSec)
	}
}

func formatETA(seconds float64) string {
	if seconds < 0 || seconds > 86400*7 {
		return "--"
	}
	secs := int(seconds)
	days := secs / 86400
	hours := (secs % 86400) / 3600
	mins := (secs % 3600) / 60
	sec := secs % 60
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm %ds", days, hours, mins, sec)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, mins, sec)
	}
	if mins > 0 {
		return fmt.Sprintf("%dm %ds", mins, sec)
	}
	return fmt.Sprintf("%ds", sec)
}

func formatDuration(d time.Duration) string {
	secs := int(d.Seconds())
	days := secs / 86400
	hours := (secs % 86400) / 3600
	mins := (secs % 3600) / 60
	sec := secs % 60
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm %ds", days, hours, mins, sec)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, mins, sec)
	}
	if mins > 0 {
		return fmt.Sprintf("%dm %ds", mins, sec)
	}
	return fmt.Sprintf("%ds", sec)
}

func formatSize(bytes int64) string {
	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
