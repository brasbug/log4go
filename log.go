package log4go

import (
	"fmt"
	"log"
	"path"
	"runtime"
	"strconv"
	"sync"
	"time"
)

var (
	LEVEL_FLAGS = [...]string{"DEBUG", "INFO","NEVERShow", "WARN", "ERROR", "FATAL"}
	recordPool  *sync.Pool
)

const (
	DEBUG = iota
	INFO
	NEVERShow
	WARNING
	ERROR
	FATAL
)

const tunnel_size_default = 1024

type Record struct {
	Time  string
	Code  string
	Info  string
	Level int
	Ext interface{}
}

func (r *Record) String() string {
	return fmt.Sprintf("%s \t [%s] \t <%s>  \t %s\n", r.Time, LEVEL_FLAGS[r.Level], r.Code, r.Info)
}

type Writer interface {
	Init() error
	Write(*Record) error
}

type Rotater interface {
	Rotate() error
	SetPathPattern(string) error
}

type Flusher interface {
	Flush() error
}

type LoggerCallbackFunc func(record Record)

type Logger struct {
	writers     []Writer
	tunnel      chan *Record
	level       int
	lastTime    int64
	lastTimeStr string
	c           chan bool
	layout      string
}


func NewLogger() *Logger {
	if logger_default != nil && takeup == false {
		takeup = true
		return logger_default
	}
	l := new(Logger)
	l.writers = make([]Writer, 0, 2)
	l.tunnel = make(chan *Record, tunnel_size_default)
	l.c = make(chan bool, 1)
	l.level = DEBUG
	l.layout = "2006-01-02 15:04:05"
	logger_default = l
	go boostrapLogWriter(l)
	return l
}

func (l *Logger) Register(w Writer) {
	if err := w.Init(); err != nil {
		panic(err)
	}
	l.writers = append(l.writers, w)
}

func (l *Logger) SetLevel(lvl int) {
	l.level = lvl
}

func (l *Logger) SetLayout(layout string) {
	l.layout = layout
}

func (l *Logger) DebugF(fmt string, args ...interface{}) {
	l.deliverRecordToWriter(DEBUG, fmt, args...)
}

func (l *Logger) WarnF(fmt string, args ...interface{}) {
	l.deliverRecordToWriter(WARNING, fmt, args...)
}

func (l *Logger) InfoF(fmt string, args ...interface{}) {
	l.deliverRecordToWriter(INFO, fmt, args...)
}

func (l *Logger) InfoFNL(fmt string, args ...interface{}) {
	l.deliverRecordToWriterCodeLine(INFO, fmt,false, args...)
}

func (l *Logger) ErrorF(fmt string, args ...interface{}) {
	l.deliverRecordToWriter(ERROR, fmt, args...)
}

func (l *Logger) FatalF(fmt string, args ...interface{}) {
	l.deliverRecordToWriter(FATAL, fmt, args...)
}

func (l *Logger) Close() {
	close(l.tunnel)
	<-l.c
	for _, w := range l.writers {
		if f, ok := w.(Flusher); ok {
			if err := f.Flush(); err != nil {
				log.Println(err)
			}
		}
	}
}

func (l *Logger) deliverRecordToWriter(level int, format string, args ...interface{}) {
	l.deliverRecordToWriterCodeLine(level,format,true,args...)
}


func (l *Logger) deliverRecordToWriterCodeLine(level int, format string,codeLine bool, args ...interface{}) {
	var inf, code string

	if level < l.level {
		return
	}

	if format != "" {
		inf = fmt.Sprintf(format, args...)
	} else {
		inf = fmt.Sprint(args...)
	}
	if codeLine {
		// source code, file and line num
		_, file, line, ok := runtime.Caller(2)
		if ok {
			code = path.Base(file) + ":" + strconv.Itoa(line)
		}
	}


	// format time
	now := time.Now()
	if now.Unix() != l.lastTime {
		l.lastTime = now.Unix()
		l.lastTimeStr = now.Format(l.layout)
	}

	r := recordPool.Get().(*Record)
	r.Info = inf
	r.Code = code
	r.Time = l.lastTimeStr
	r.Level = level
	l.tunnel <- r
}


func (l *Logger) deliverRecordExtToWriter(ext interface{},level int, format string, args ...interface{}) {
	var inf, code string

	if level < l.level {
		return
	}
	if format != "" {
		inf = fmt.Sprintf(format, args...)
	} else {
		inf = fmt.Sprint(args...)
	}

	// source code, file and line num
	_, file, line, ok := runtime.Caller(2)
	if ok {
		code = path.Base(file) + ":" + strconv.Itoa(line)
	}

	// format time
	now := time.Now()
	if now.Unix() != l.lastTime {
		l.lastTime = now.Unix()
		l.lastTimeStr = now.Format(l.layout)
	}

	r := recordPool.Get().(*Record)
	r.Info = inf
	r.Code = code
	r.Time = l.lastTimeStr
	r.Level = level
	r.Ext = ext
	l.tunnel <- r
}

func boostrapLogWriter(logger *Logger) {
	if logger == nil {
		panic("logger is nil")
	}

	var (
		r  *Record
		ok bool
	)

	if r, ok = <-logger.tunnel; !ok {
		logger.c <- true
		return
	}

	for _, w := range logger.writers {
		if err := w.Write(r); err != nil {
			log.Println(err)
		}
	}

	flushTimer := time.NewTimer(time.Millisecond * 500)
	rotateTimer := time.NewTimer(time.Second * 10)

	for {
		select {
		case r, ok = <-logger.tunnel:
			if !ok {
				logger.c <- true
				return
			}
			for _, w := range logger.writers {
				if err := w.Write(r); err != nil {
					log.Println(err)
				}
			}

			recordPool.Put(r)

		case <-flushTimer.C:
			for _, w := range logger.writers {
				if f, ok := w.(Flusher); ok {
					if err := f.Flush(); err != nil {
						log.Println(err)
					}
				}
			}
			flushTimer.Reset(time.Millisecond * 1000)

		case <-rotateTimer.C:
			for _, w := range logger.writers {
				if r, ok := w.(Rotater); ok {
					if err := r.Rotate(); err != nil {
						log.Println(err)
					}
				}
			}
			rotateTimer.Reset(time.Second * 10)
		}
	}
}

// default
var (
	logger_default *Logger
	takeup         = false
)

func SetLevel(lvl int) {
	logger_default.level = lvl
}

func SetLayout(layout string) {
	logger_default.layout = layout
}


func Debug(args ...interface{}) {
	logger_default.deliverRecordToWriter(DEBUG, "", args...)
}

func Warn(args ...interface{}) {
	logger_default.deliverRecordToWriter(WARNING, "", args...)
}

func Info(args ...interface{}) {
	logger_default.deliverRecordToWriter(INFO, "", args...)
}

func Error(args ...interface{}) {
	logger_default.deliverRecordToWriter(ERROR, "", args...)
}

func Fatal(args ...interface{}) {
	logger_default.deliverRecordToWriter(FATAL, "", args...)
}


func DebugF(fmt string, args ...interface{}) {
	logger_default.deliverRecordToWriter(DEBUG, fmt, args...)
}

func WarnF(fmt string, args ...interface{}) {
	logger_default.deliverRecordToWriter(WARNING, fmt, args...)
}

func InfoF(fmt string, args ...interface{}) {
	logger_default.deliverRecordToWriter(INFO, fmt, args...)
}

func InfoFNL(fmt string, args ...interface{}) {
	logger_default.deliverRecordToWriterCodeLine(INFO, fmt,false, args...)
}

func ErrorF(fmt string, args ...interface{}) {
	logger_default.deliverRecordToWriter(ERROR, fmt, args...)
}

func FatalF(fmt string, args ...interface{}) {
	logger_default.deliverRecordToWriter(FATAL, fmt, args...)
}


func DebugExtF(ext interface{},fmt string, args ...interface{}) {
	logger_default.deliverRecordExtToWriter(ext, DEBUG, fmt, args...)
}

func WarnExtF(ext interface{},fmt string, args ...interface{}) {
	logger_default.deliverRecordExtToWriter(ext, WARNING, fmt, args...)
}

func InfoExtF(ext interface{},fmt string, args ...interface{}) {
	logger_default.deliverRecordExtToWriter(ext, INFO, fmt, args...)
}

func NeverShowF(ext interface{},fmt string, args ...interface{}) {
	logger_default.deliverRecordExtToWriter(ext, NEVERShow, fmt, args...)
}
func ErrorExtF(ext interface{},fmt string, args ...interface{}) {
	logger_default.deliverRecordExtToWriter(ext, ERROR, fmt, args...)
}

func FatalExtF(ext interface{},fmt string, args ...interface{}) {
	logger_default.deliverRecordExtToWriter(ext, FATAL, fmt, args...)
}


func Register(w Writer) {
	logger_default.Register(w)
}

func Close() {
	logger_default.Close()
}

func init() {
	logger_default = NewLogger()
	recordPool = &sync.Pool{New: func() interface{} {
		return &Record{}
	}}
}
