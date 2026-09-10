package server

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/BearHero520/miair-plus/internal/config"
)

const maxAlarmUpload = 100 << 20

var nasVolume = regexp.MustCompile(`^/vol[1-9][0-9]*/`)

func allowedNASAudio(path string) bool {
	if !nasVolume.MatchString(filepath.ToSlash(path)) || alarmFormats[strings.ToLower(filepath.Ext(path))] == "" {
		return false
	}
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if strings.HasPrefix(part, "@") || strings.HasPrefix(part, ".") {
			return false
		}
	}
	return true
}
func (e *AlarmEngine) saveImport(input io.Reader, name string) (string, error) {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	if alarmFormats[strings.ToLower(filepath.Ext(name))] == "" {
		return "", errors.New("支持 MP3、WAV、FLAC、OGG、AAC")
	}
	if len(name) > 180 || strings.ContainsAny(name, "\r\n\x00") {
		return "", errors.New("文件名过长或无效")
	}
	if err := os.MkdirAll(e.root, 0700); err != nil {
		return "", errors.New("铃声目录不可写，请检查应用文件权限")
	}
	f, err := os.CreateTemp(e.root, ".upload-*")
	if err != nil {
		return "", errors.New("无法保存铃声，请检查磁盘空间和权限")
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	size, err := io.Copy(f, io.LimitReader(input, maxAlarmUpload+1))
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return "", err
	}
	if closeErr != nil {
		return "", closeErr
	}
	if size == 0 || size > maxAlarmUpload {
		return "", errors.New("音频文件需大于 0 且不超过 100 MB")
	}
	sound := config.Random(6) + "-" + name
	if err = os.Rename(tmp, filepath.Join(e.root, sound)); err != nil {
		return "", err
	}
	return sound, nil
}
func (a *API) uploadAlarm(w http.ResponseWriter, r *http.Request) {
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(3 * time.Minute))
	e := a.Manager.alarms
	select {
	case e.importing <- struct{}{}:
		defer func() { <-e.importing }()
	default:
		fail(w, 409, errors.New("正在处理铃声，请稍后再试"))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxAlarmUpload+(1<<20))
	reader, err := r.MultipartReader()
	if err != nil {
		fail(w, 400, errors.New("请选择一个音频文件"))
		return
	}
	part, err := reader.NextPart()
	if err != nil {
		fail(w, 400, errors.New("上传内容为空"))
		return
	}
	defer part.Close()
	if part.FormName() != "file" || part.FileName() == "" {
		fail(w, 400, errors.New("请选择一个音频文件"))
		return
	}
	sound, err := e.saveImport(part, part.FileName())
	if err != nil {
		fail(w, 400, err)
		return
	}
	if _, err = reader.NextPart(); err != io.EOF {
		os.Remove(filepath.Join(e.root, sound))
		fail(w, 400, errors.New("一次只能上传一个音频文件"))
		return
	}
	jsonOut(w, map[string]string{"sound": sound})
}
func (e *AlarmEngine) ImportNAS(path string) (string, error) {
	select {
	case e.importing <- struct{}{}:
		defer func() { <-e.importing }()
	default:
		return "", errors.New("正在处理铃声，请稍后再试")
	}
	if !allowedNASAudio(path) {
		return "", errors.New("请选择 NAS 存储空间中的音频文件")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || !allowedNASAudio(resolved) {
		return "", errors.New("所选文件路径不可访问")
	}
	f, err := os.Open(resolved)
	if err != nil {
		return "", errors.New("尚未获得此文件的读取权限，请通过飞牛文件选择器授权，或使用上传音乐")
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Size() > maxAlarmUpload {
		return "", fmt.Errorf("请选择 100 MB 以内的普通音频文件")
	}
	return e.saveImport(f, filepath.Base(path))
}
