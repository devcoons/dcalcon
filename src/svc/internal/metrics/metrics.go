package metrics

import (
	"fmt"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
)

var (
	AuthFailures atomic.Int64
	AuthLockouts atomic.Int64
	IMIPSent     atomic.Int64
	IMIPErrors   atomic.Int64
	BackupOK     atomic.Int64
	BackupErrors atomic.Int64
	ScheduleErr  atomic.Int64

	davMu sync.Mutex
	dav   = map[string]int64{}
)

func IncAuthFail()      { AuthFailures.Add(1) }
func IncAuthLockout()   { AuthLockouts.Add(1) }
func IncIMIPSent()      { IMIPSent.Add(1) }
func IncIMIPError()     { IMIPErrors.Add(1) }
func IncBackupOK()      { BackupOK.Add(1) }
func IncBackupError()   { BackupErrors.Add(1) }
func IncScheduleError() { ScheduleErr.Add(1) }

func IncDAV(method string) {
	if method == "" {
		method = "UNKNOWN"
	}
	davMu.Lock()
	dav[method]++
	davMu.Unlock()
}

func Write(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprintf(w, "# HELP dcalcon_auth_failures_total Failed web logins and DAV Basic attempts\n")
	fmt.Fprintf(w, "# TYPE dcalcon_auth_failures_total counter\n")
	fmt.Fprintf(w, "dcalcon_auth_failures_total %d\n", AuthFailures.Load())
	fmt.Fprintf(w, "# HELP dcalcon_auth_lockouts_total Rate-limit lockouts\n")
	fmt.Fprintf(w, "# TYPE dcalcon_auth_lockouts_total counter\n")
	fmt.Fprintf(w, "dcalcon_auth_lockouts_total %d\n", AuthLockouts.Load())
	fmt.Fprintf(w, "# HELP dcalcon_imip_sent_total External iMIP messages accepted by a provider\n")
	fmt.Fprintf(w, "# TYPE dcalcon_imip_sent_total counter\n")
	fmt.Fprintf(w, "dcalcon_imip_sent_total %d\n", IMIPSent.Load())
	fmt.Fprintf(w, "# HELP dcalcon_imip_errors_total External iMIP send failures\n")
	fmt.Fprintf(w, "# TYPE dcalcon_imip_errors_total counter\n")
	fmt.Fprintf(w, "dcalcon_imip_errors_total %d\n", IMIPErrors.Load())
	fmt.Fprintf(w, "# HELP dcalcon_backup_success_total SQLite VACUUM INTO backups that finished\n")
	fmt.Fprintf(w, "# TYPE dcalcon_backup_success_total counter\n")
	fmt.Fprintf(w, "dcalcon_backup_success_total %d\n", BackupOK.Load())
	fmt.Fprintf(w, "# HELP dcalcon_backup_errors_total Failed backups\n")
	fmt.Fprintf(w, "# TYPE dcalcon_backup_errors_total counter\n")
	fmt.Fprintf(w, "dcalcon_backup_errors_total %d\n", BackupErrors.Load())
	fmt.Fprintf(w, "# HELP dcalcon_schedule_errors_total Local invite delivery failures after a calendar PUT\n")
	fmt.Fprintf(w, "# TYPE dcalcon_schedule_errors_total counter\n")
	fmt.Fprintf(w, "dcalcon_schedule_errors_total %d\n", ScheduleErr.Load())
	fmt.Fprintf(w, "# HELP dcalcon_dav_requests_total DAV requests by HTTP method\n")
	fmt.Fprintf(w, "# TYPE dcalcon_dav_requests_total counter\n")
	davMu.Lock()
	methods := make([]string, 0, len(dav))
	for method := range dav {
		methods = append(methods, method)
	}
	sort.Strings(methods)
	for _, method := range methods {
		fmt.Fprintf(w, "dcalcon_dav_requests_total{method=%q} %d\n", method, dav[method])
	}
	davMu.Unlock()
}
