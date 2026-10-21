package sshmgr

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

var ErrHostNotFound = errors.New("操作机不存在")

type hostStatus struct {
	State       string
	LatencyMS   int64
	LastChecked time.Time
	LastError   string
	Fingerprint string
}

type Manager struct {
	baseDir string
	path    string

	mu      sync.RWMutex
	cfg     fileConfig
	loadErr error
	status  map[string]hostStatus
	probe   Prober

	startOnce sync.Once
	stopOnce  sync.Once
	stop      chan struct{}
	wake      chan struct{}
	scanMu    sync.Mutex
}

func NewManager(baseDir string) *Manager {
	path := configPath(baseDir)
	cfg, err := loadConfig(path)
	manager := &Manager{
		baseDir: baseDir,
		path:    path,
		cfg:     cfg,
		loadErr: err,
		status:  map[string]hostStatus{},
		probe:   NewDialer(baseDir),
		stop:    make(chan struct{}),
		wake:    make(chan struct{}, 1),
	}
	for _, host := range cfg.Hosts {
		manager.status[host.ID] = hostStatus{State: StateUnknown}
	}
	return manager
}

func (m *Manager) ConfigPath() string {
	return "configs/" + configFileName
}

func (m *Manager) Start() {
	if m == nil {
		return
	}
	m.mu.RLock()
	loadErr := m.loadErr
	m.mu.RUnlock()
	if loadErr != nil {
		return
	}
	m.startOnce.Do(func() {
		go m.loop()
	})
}

func (m *Manager) Close() {
	if m == nil {
		return
	}
	m.stopOnce.Do(func() {
		close(m.stop)
	})
}

func (m *Manager) ScanNow() {
	if m == nil {
		return
	}
	m.notify()
}

func (m *Manager) notify() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

func (m *Manager) loop() {
	m.scanAll()
	for {
		timer := time.NewTimer(m.interval())
		select {
		case <-m.stop:
			timer.Stop()
			return
		case <-m.wake:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			m.scanAll()
		case <-timer.C:
			m.scanAll()
		}
	}
}

func (m *Manager) Overview() (Overview, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.loadErr != nil {
		return Overview{}, m.loadErr
	}
	hosts := make([]HostView, 0, len(m.cfg.Hosts))
	for _, host := range m.cfg.Hosts {
		hosts = append(hosts, m.viewLocked(host))
	}
	return Overview{
		IntervalSeconds: m.cfg.IntervalSeconds,
		TimeoutSeconds:  m.cfg.TimeoutSeconds,
		ConfigPath:      m.ConfigPath(),
		Hosts:           hosts,
	}, nil
}

func (m *Manager) Create(input HostInput) (HostView, error) {
	m.mu.Lock()
	if m.loadErr != nil {
		err := m.loadErr
		m.mu.Unlock()
		return HostView{}, err
	}
	host, err := buildHost(input, nil)
	if err != nil {
		m.mu.Unlock()
		return HostView{}, err
	}
	host.ID = newHostID(m.cfg.Hosts)
	next := m.cfg.clone()
	next.Hosts = append(next.Hosts, host)
	if err := saveConfig(m.path, next); err != nil {
		m.mu.Unlock()
		return HostView{}, err
	}
	m.cfg = next
	m.loadErr = nil
	m.status[host.ID] = hostStatus{State: StateUnknown}
	view := m.viewLocked(host)
	m.mu.Unlock()
	m.notify()
	return view, nil
}

func (m *Manager) Update(id string, input HostInput) (HostView, error) {
	m.mu.Lock()
	if m.loadErr != nil {
		err := m.loadErr
		m.mu.Unlock()
		return HostView{}, err
	}
	index := -1
	for current := range m.cfg.Hosts {
		if m.cfg.Hosts[current].ID == id {
			index = current
			break
		}
	}
	if index < 0 {
		m.mu.Unlock()
		return HostView{}, fmt.Errorf("%w: %s", ErrHostNotFound, id)
	}
	existing := m.cfg.Hosts[index]
	host, err := buildHost(input, &existing)
	if err != nil {
		m.mu.Unlock()
		return HostView{}, err
	}
	host.ID = existing.ID
	next := m.cfg.clone()
	next.Hosts[index] = host
	if err := saveConfig(m.path, next); err != nil {
		m.mu.Unlock()
		return HostView{}, err
	}
	m.cfg = next
	m.loadErr = nil
	m.status[host.ID] = hostStatus{State: StateUnknown}
	view := m.viewLocked(host)
	m.mu.Unlock()
	m.notify()
	return view, nil
}

func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	if m.loadErr != nil {
		err := m.loadErr
		m.mu.Unlock()
		return err
	}
	index := -1
	for current := range m.cfg.Hosts {
		if m.cfg.Hosts[current].ID == id {
			index = current
			break
		}
	}
	if index < 0 {
		m.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrHostNotFound, id)
	}
	next := m.cfg.clone()
	next.Hosts = append(next.Hosts[:index], next.Hosts[index+1:]...)
	if err := saveConfig(m.path, next); err != nil {
		m.mu.Unlock()
		return err
	}
	m.cfg = next
	delete(m.status, id)
	m.mu.Unlock()
	m.notify()
	return nil
}

func (m *Manager) UpdateSettings(input SettingsInput) error {
	if err := validateSettings(input); err != nil {
		return err
	}
	m.mu.Lock()
	if m.loadErr != nil {
		err := m.loadErr
		m.mu.Unlock()
		return err
	}
	next := m.cfg.clone()
	next.IntervalSeconds = input.IntervalSeconds
	next.TimeoutSeconds = input.TimeoutSeconds
	if err := saveConfig(m.path, next); err != nil {
		m.mu.Unlock()
		return err
	}
	m.cfg = next
	m.mu.Unlock()
	m.notify()
	return nil
}

func (m *Manager) Check(ctx context.Context, id string) (HostView, error) {
	host, timeout, err := m.hostForCheck(id)
	if err != nil {
		return HostView{}, err
	}
	m.setChecking(host.ID)
	result := m.probeHost(ctx, host, timeout)
	m.setStatus(host.ID, result)
	return m.hostView(host), nil
}

func (m *Manager) Exec(ctx context.Context, id, command string) (ExecResult, error) {
	host, timeout, err := m.hostForCheck(id)
	if err != nil {
		return ExecResult{}, err
	}
	result, execErr := m.probe.Exec(ctx, host, timeout, command)
	status := hostStatus{
		State:       StateOnline,
		LatencyMS:   result.LatencyMS,
		LastChecked: time.Now(),
		Fingerprint: result.Fingerprint,
	}
	if execErr != nil {
		status.State = StateOffline
		status.LastError = execErr.Error()
	}
	m.setStatus(host.ID, status)
	return result, execErr
}

func (m *Manager) StreamExec(ctx context.Context, id, command string, stdin io.Reader, stdout, stderr io.Writer) (StreamResult, error) {
	host, timeout, err := m.hostForCheck(id)
	if err != nil {
		return StreamResult{}, err
	}
	result, streamErr := m.probe.Stream(ctx, host, timeout, command, stdin, stdout, stderr)
	status := hostStatus{
		State:       StateOnline,
		LatencyMS:   result.LatencyMS,
		LastChecked: time.Now(),
		Fingerprint: result.Fingerprint,
	}
	if streamErr != nil && !errors.Is(streamErr, context.Canceled) && !errors.Is(streamErr, context.DeadlineExceeded) {
		status.State = StateOffline
		status.LastError = streamErr.Error()
	}
	m.setStatus(host.ID, status)
	return result, streamErr
}

func (m *Manager) HostByID(id string) (Host, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.loadErr != nil {
		return Host{}, m.loadErr
	}
	for _, host := range m.cfg.Hosts {
		if host.ID == id {
			return host, nil
		}
	}
	return Host{}, fmt.Errorf("%w: %s", ErrHostNotFound, id)
}

func (m *Manager) hostForCheck(id string) (Host, time.Duration, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.loadErr != nil {
		return Host{}, 0, m.loadErr
	}
	for _, host := range m.cfg.Hosts {
		if host.ID == id {
			return host, time.Duration(m.cfg.TimeoutSeconds) * time.Second, nil
		}
	}
	return Host{}, 0, fmt.Errorf("%w: %s", ErrHostNotFound, id)
}

func (m *Manager) probeHost(ctx context.Context, host Host, timeout time.Duration) hostStatus {
	result, err := m.probe.Check(ctx, host, timeout)
	status := hostStatus{
		State:       StateOnline,
		LatencyMS:   result.Latency.Milliseconds(),
		LastChecked: time.Now(),
		Fingerprint: result.Fingerprint,
	}
	if err != nil {
		status.State = StateOffline
		status.LastError = err.Error()
	}
	return status
}

func (m *Manager) scanAll() {
	if m == nil || !m.scanMu.TryLock() {
		return
	}
	defer m.scanMu.Unlock()

	m.mu.RLock()
	if m.loadErr != nil {
		m.mu.RUnlock()
		return
	}
	timeout := time.Duration(m.cfg.TimeoutSeconds) * time.Second
	hosts := make([]Host, 0, len(m.cfg.Hosts))
	for _, host := range m.cfg.Hosts {
		if host.IsEnabled() {
			hosts = append(hosts, host)
		}
	}
	m.mu.RUnlock()

	m.mu.Lock()
	for _, host := range hosts {
		previous := m.status[host.ID]
		m.status[host.ID] = hostStatus{
			State:       StateChecking,
			LatencyMS:   previous.LatencyMS,
			LastChecked: previous.LastChecked,
			Fingerprint: previous.Fingerprint,
		}
	}
	m.mu.Unlock()

	semaphore := make(chan struct{}, 4)
	var waitGroup sync.WaitGroup
	for _, host := range hosts {
		waitGroup.Add(1)
		go func(item Host) {
			defer waitGroup.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			m.setStatus(item.ID, m.probeHost(context.Background(), item, timeout))
		}(host)
	}
	waitGroup.Wait()

	m.mu.Lock()
	valid := make(map[string]bool, len(m.cfg.Hosts))
	for _, host := range m.cfg.Hosts {
		valid[host.ID] = true
	}
	for id := range m.status {
		if !valid[id] {
			delete(m.status, id)
		}
	}
	m.mu.Unlock()
}

func (m *Manager) interval() time.Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.cfg.IntervalSeconds < MinIntervalSeconds {
		return DefaultIntervalSeconds * time.Second
	}
	return time.Duration(m.cfg.IntervalSeconds) * time.Second
}

func (m *Manager) setChecking(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	previous := m.status[id]
	previous.State = StateChecking
	previous.LastError = ""
	m.status[id] = previous
}

func (m *Manager) setStatus(id string, status hostStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status[id] = status
}

func (m *Manager) hostView(host Host) HostView {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.viewLocked(host)
}

func (m *Manager) viewLocked(host Host) HostView {
	status := m.status[host.ID]
	state := status.State
	if state == "" {
		state = StateUnknown
	}
	view := HostView{
		ID:             host.ID,
		Name:           host.Name,
		Address:        host.Address,
		Port:           host.Port,
		User:           host.User,
		Auth:           host.Auth,
		KeyPath:        host.KeyPath,
		KnownHostsPath: host.KnownHostsPath,
		BundleBaseDir:  host.BundleBaseDir,
		KeepBundle:     host.KeepBundle,
		StrictHostKey:  host.IsStrictHostKey(),
		Enabled:        host.IsEnabled(),
		Description:    host.Description,
		HasPassword:    host.Password != "",
		HasPassphrase:  host.Passphrase != "",
		Status:         state,
		LatencyMS:      status.LatencyMS,
		LastError:      truncateText(status.LastError, 800),
		Fingerprint:    status.Fingerprint,
	}
	if !status.LastChecked.IsZero() {
		view.LastChecked = status.LastChecked.Format(time.RFC3339)
	}
	return view
}

func truncateText(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "..."
}
