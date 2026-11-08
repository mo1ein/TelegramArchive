// Package cron manages ta's entries in the current user's crontab.
package cron

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
)

type Job struct {
	Name        string            `json:"name"`
	Schedule    string            `json:"schedule"`
	Executable  string            `json:"executable"`
	Directory   string            `json:"directory"`
	Arguments   []string          `json:"arguments"`
	Environment map[string]string `json:"environment"`
	Log         string            `json:"log"`
	Installed   bool              `json:"-"`
}

type Table interface {
	Load(context.Context) (string, error)
	Save(context.Context, string) error
}

type Manager struct {
	Directory string
	Table     Table
}

var validName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

func ValidateName(name string) error {
	if !validName.MatchString(name) {
		return fmt.Errorf("job name must contain 1–64 letters, numbers, underscores or hyphens, starting with a letter or number")
	}
	return nil
}

func Schedule(every time.Duration) (string, error) {
	if every <= 0 || every%time.Minute != 0 {
		return "", fmt.Errorf("cron intervals must be whole minutes; use 30m, 1h, 6h or 24h")
	}
	minutes := int64(every / time.Minute)
	switch {
	case minutes == 1:
		return "* * * * *", nil
	case minutes < 60 && 60%minutes == 0:
		return fmt.Sprintf("*/%d * * * *", minutes), nil
	case minutes == 60:
		return "0 * * * *", nil
	case minutes < 1440 && minutes%60 == 0 && 24%(minutes/60) == 0:
		return fmt.Sprintf("0 */%d * * *", minutes/60), nil
	case minutes == 1440:
		return "0 0 * * *", nil
	}
	return "", fmt.Errorf("%s cannot repeat evenly in cron; use minutes dividing an hour, hours dividing a day, or 24h (for other intervals use ta sync --every)", every)
}

func (m Manager) File(name string) string { return filepath.Join(m.Directory, name+".json") }

func quote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }

func (m Manager) Line(job Job) (string, error) {
	if err := ValidateName(job.Name); err != nil {
		return "", err
	}
	for _, path := range []string{job.Executable, m.File(job.Name)} {
		if !filepath.IsAbs(path) || strings.ContainsAny(path, "%\r\n\x00") {
			return "", fmt.Errorf("cron executable and configuration paths must be absolute and cannot contain percent signs or newlines")
		}
	}
	if len(strings.Fields(job.Schedule)) != 5 || strings.ContainsAny(job.Schedule, "\r\n") {
		return "", fmt.Errorf("invalid cron schedule")
	}
	return job.Schedule + " " + quote(job.Executable) + " cron run --file " + quote(m.File(job.Name)) + " > /dev/null 2>&1", nil
}

func begin(name string) string { return "# ta cron begin " + name }
func end(name string) string   { return "# ta cron end " + name }

func replace(table, name, line string) (string, error) {
	lines := strings.SplitAfter(table, "\n")
	var out strings.Builder
	for i := 0; i < len(lines); i++ {
		if strings.TrimSuffix(strings.TrimSuffix(lines[i], "\n"), "\r") != begin(name) {
			out.WriteString(lines[i])
			continue
		}
		if i+2 >= len(lines) || strings.TrimSpace(lines[i+2]) != end(name) {
			return "", fmt.Errorf("cron block for %q is damaged; leaving your crontab unchanged", name)
		}
		i += 2
	}
	if line != "" {
		if out.Len() > 0 && !strings.HasSuffix(out.String(), "\n") {
			out.WriteByte('\n')
		}
		fmt.Fprintf(&out, "%s\n%s\n%s\n", begin(name), line, end(name))
	}
	return out.String(), nil
}

func writeFile(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".cron-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func (m Manager) lock() (*bolt.DB, error) {
	if err := os.MkdirAll(m.Directory, 0700); err != nil {
		return nil, err
	}
	if err := os.Chmod(m.Directory, 0700); err != nil {
		return nil, err
	}
	return bolt.Open(filepath.Join(m.Directory, ".manage.lock"), 0600, &bolt.Options{Timeout: time.Second})
}

func (m Manager) Install(ctx context.Context, job Job) error {
	line, err := m.Line(job)
	if err != nil {
		return err
	}
	lock, err := m.lock()
	if err != nil {
		return err
	}
	defer lock.Close()
	previous, err := m.Table.Load(ctx)
	if err != nil {
		return err
	}
	next, err := replace(previous, job.Name, line)
	if err != nil {
		return err
	}
	path := m.File(job.Name)
	old, readErr := os.ReadFile(path)
	if readErr != nil && !os.IsNotExist(readErr) {
		return readErr
	}
	b, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		return err
	}
	if err = writeFile(path, append(b, '\n')); err != nil {
		return err
	}
	if err = m.Table.Save(ctx, next); err != nil {
		var rollback error
		if readErr == nil {
			rollback = writeFile(path, old)
		} else {
			rollback = os.Remove(path)
		}
		return errors.Join(err, rollback)
	}
	return nil
}

func (m Manager) Remove(ctx context.Context, name string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	lock, err := m.lock()
	if err != nil {
		return err
	}
	defer lock.Close()
	previous, err := m.Table.Load(ctx)
	if err != nil {
		return err
	}
	next, err := replace(previous, name, "")
	if err != nil {
		return err
	}
	if next == previous {
		return fmt.Errorf("no installed ta cron job named %q", name)
	}
	if err = m.Table.Save(ctx, next); err != nil {
		return err
	}
	if err = os.Remove(m.File(name)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func ReadJob(path string) (Job, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Job{}, err
	}
	var job Job
	if err = json.Unmarshal(b, &job); err != nil {
		return job, err
	}
	if err = ValidateName(job.Name); err != nil {
		return job, err
	}
	if !filepath.IsAbs(job.Executable) || !filepath.IsAbs(job.Directory) || !filepath.IsAbs(job.Log) {
		return job, fmt.Errorf("job contains a relative executable, working directory or log path")
	}
	return job, nil
}

func (m Manager) List(ctx context.Context) ([]Job, error) {
	table, err := m.Table.Load(ctx)
	if err != nil {
		return nil, err
	}
	files, err := filepath.Glob(filepath.Join(m.Directory, "*.json"))
	if err != nil {
		return nil, err
	}
	var jobs []Job
	for _, file := range files {
		job, err := ReadJob(file)
		if err != nil {
			return nil, fmt.Errorf("read job %s: %w", filepath.Base(file), err)
		}
		job.Installed = strings.Contains("\n"+table, "\n"+begin(job.Name)+"\n")
		jobs = append(jobs, job)
	}
	return jobs, nil
}
