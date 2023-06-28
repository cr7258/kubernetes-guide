package utils

import "log"

type MyLogger struct {
	Debug bool
}
func (logger MyLogger) Debugf(format string, args ...interface{}) {
	if logger.Debug {
		log.Printf(format+"\n", args...)
	}
}

// Log to stdout only if Debug is true.
func (logger MyLogger) Infof(format string, args ...interface{}) {
	if logger.Debug {
		log.Printf(format+"\n", args...)
	}
}

// Log to stdout always.
func (logger MyLogger) Warnf(format string, args ...interface{}) {
	log.Printf(format+"\n", args...)
}

// Log to stdout always.
func (logger MyLogger) Errorf(format string, args ...interface{}) {
	log.Printf(format+"\n", args...)
}