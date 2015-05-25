package writer

import (
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/CIRCL/pbtc/adaptor"
	"github.com/CIRCL/pbtc/compressor"
)

const Version = "PBTC Log Version 1"

type FileWriter struct {
	comp adaptor.Compressor
	log  adaptor.Log

	filePath  string
	fileSize  int64
	fileAge   time.Duration
	fileTimer *time.Timer
	file      *os.File

	sigWriter chan struct{}
	wg        *sync.WaitGroup
	txtQ      chan string

	done uint32
}

func NewFileWriter(options ...func(*FileWriter)) (*FileWriter, error) {
	w := &FileWriter{
		filePath: "logs/",
		fileSize: 1 * 1024 * 1024,
		fileAge:  1 * 60 * time.Minute,

		sigWriter: make(chan struct{}),
		wg:        &sync.WaitGroup{},
		txtQ:      make(chan string, 1),
	}

	for _, option := range options {
		option(w)
	}

	if w.comp == nil {
		w.comp = compressor.NewDummy()
	}

	_, err := os.Stat(w.filePath)
	if err != nil {
		err := os.MkdirAll(w.filePath, 0777)
		if err != nil {
			return nil, err
		}
	}

	w.rotateLog()

	w.fileTimer = time.NewTimer(w.fileAge)

	w.startup()

	return w, nil
}

func SetLog(log adaptor.Log) func(*FileWriter) {
	return func(w *FileWriter) {
		w.log = log
	}
}

// SetCompressor injects the compression wrapper to be used on rotation.
func SetCompressor(comp adaptor.Compressor) func(*FileWriter) {
	return func(w *FileWriter) {
		w.comp = comp
	}
}

// SetFilePath sets the directory path to the files into.
func SetFilePath(path string) func(*FileWriter) {
	return func(w *FileWriter) {
		w.filePath = path
	}
}

// SetSizeLimit sets the size limit upon which the logs will rotate.
func SetSizeLimit(size int64) func(*FileWriter) {
	return func(w *FileWriter) {
		w.fileSize = size
	}
}

// SetAgeLimit sets the file age upon which the logs will rotate.
func SetAgeLimit(age time.Duration) func(*FileWriter) {
	return func(w *FileWriter) {
		w.fileAge = age
	}
}

func (w *FileWriter) SetLog(log adaptor.Log) {
	w.log = log
}

// Stop ends the execution of the recorder sub-routines and returns once
// everything was stopped cleanly.
func (w *FileWriter) Stop() {
	if atomic.SwapUint32(&w.done, 1) == 1 {
		return
	}

	close(w.sigWriter)

	w.wg.Wait()
}

func (w *FileWriter) Line(line string) {
	w.txtQ <- line
}

func (w *FileWriter) startup() {
	w.wg.Add(1)
	go w.goWriter()
}

func (w *FileWriter) goWriter() {
	defer w.wg.Done()

WriteLoop:
	for {
		select {
		case _, ok := <-w.sigWriter:
			if !ok {
				break WriteLoop
			}

		case <-w.fileTimer.C:
			w.checkTime()

		case txt := <-w.txtQ:
			_, err := w.file.WriteString(txt + "\n")
			if err != nil {
				w.log.Error("[REC] Could not write txt file (%v)", err)
			}

			w.checkSize()
		}
	}

	w.file.Close()
}

func (w *FileWriter) checkTime() {
	if w.fileAge == 0 {
		return
	}

	w.rotateLog()

	w.fileTimer.Reset(w.fileAge)
}

func (w *FileWriter) checkSize() {
	if w.fileSize == 0 {
		return
	}

	fileStat, err := w.file.Stat()
	if err != nil {
		panic(err)
	}

	if fileStat.Size() < w.fileSize {
		return
	}

	w.rotateLog()
}

func (w *FileWriter) rotateLog() {
	file, err := os.Create(w.filePath +
		time.Now().Format(time.RFC3339) + ".txt")
	if err != nil {
		return
	}

	_, err = file.WriteString("#" + Version + "\n")
	if err != nil {
		return
	}

	if w.file != nil {
		w.compressLog()
		err = w.file.Close()
		if err != nil {
			w.log.Warning("[REC] Could not close file on rotate (%v)", err)
		}
	}

	w.file = file
}

func (w *FileWriter) compressLog() {
	_, err := w.file.Seek(0, 0)
	if err != nil {
		w.log.Warning("[REC] Failed to seek output file (%v)", err)
		return
	}

	output, err := os.Create(w.file.Name() + ".out")
	if err != nil {
		w.log.Critical("[REC] Failed to create output file (%v)", err)
		return
	}

	writer, err := w.comp.GetWriter(output)
	if err != nil {
		w.log.Error("[REC] Failed to create output writer (%v)", err)
		return
	}

	_, err = io.Copy(writer, w.file)
	if err != nil {
		w.log.Error("[REC] Failed to compress log file (%v)", err)
		return
	}
}
