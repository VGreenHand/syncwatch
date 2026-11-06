import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import './App.css'
import {
  ChooseVideo,
  ClearLogs,
  CopyText,
  CreateRoom,
  GetConfig,
  GetLogs,
  DebugIntent,
  GetState,
  JoinRoom,
  LeaveRoom,
  LogMsg,
  LoadVideo,
  PingServer,
  SaveConfig,
  Seek,
  SetRate,
  SetVideoRect,
  SetVolume,
  ShowVideoHost,
  ToggleFullscreen,
  TogglePlay,
} from '../wailsjs/go/main/App'
import { EventsOn } from '../wailsjs/runtime/runtime'
import {
  IconAlert, IconArrowRight, IconBack, IconCheck, IconClose, IconCopy,
  IconExitFullscreen, IconFilm, IconFullscreen, IconLink, IconLog,
  IconMute, IconPause, IconPlay, IconPlus, IconSettings, IconUsers, IconVolume,
} from './Icons'

type Page = 'home' | 'create' | 'join' | 'player'
type Panel = null | 'members' | 'settings' | 'logs'

type Member = { name: string; state: string; file?: string; duration?: number; online: boolean }
type Video = { path: string; name: string; size: number }
type Notice = { id: number; text: string; kind: 'info' | 'warn' | 'error' }

const fmtTime = (s: number) => {
  if (!isFinite(s) || s < 0) s = 0
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const sec = Math.floor(s % 60)
  const mm = String(m).padStart(2, '0')
  const ss = String(sec).padStart(2, '0')
  return h > 0 ? `${h}:${mm}:${ss}` : `${mm}:${ss}`
}

const fmtSize = (n: number) => {
  if (!n) return ''
  if (n > 1024 ** 3) return (n / 1024 ** 3).toFixed(1) + ' GB'
  if (n > 1024 ** 2) return (n / 1024 ** 2).toFixed(0) + ' MB'
  return (n / 1024).toFixed(0) + ' KB'
}

const stateCn = (s: string) => {
  switch (s) {
    case 'playing': return '正在播放'
    case 'paused': return '已暂停'
    case 'buffering': return '正在缓冲'
    default: return '在线'
  }
}

const stateDot = (s: string) => {
  switch (s) {
    case 'playing': return 'dot-play'
    case 'paused': return 'dot-pause'
    case 'buffering': return 'dot-buffer'
    default: return 'dot-idle'
  }
}

export default function App() {
  const [page, setPage] = useState<Page>('home')
  const [panel, setPanel] = useState<Panel>(null)

  const [server, setServer] = useState('')
  const [name, setName] = useState('')
  const [video, setVideo] = useState<Video | null>(null)
  const [roomInput, setRoomInput] = useState('')
  const [password, setPassword] = useState('')

  const [room, setRoom] = useState('')
  const [conn, setConn] = useState('idle')
  const [members, setMembers] = useState<Member[]>([])
  const [pos, setPos] = useState(0)
  const [dur, setDur] = useState(0)
  const [paused, setPaused] = useState(true)
  const [rate, setRate] = useState(1)
  const [volume, setVolume] = useState(100)
  const [full, setFull] = useState(false)
  const [scrubbing, setScrubbing] = useState(false)
  const [ctrlVisible, setCtrlVisible] = useState(true)

  const [notices, setNotices] = useState<Notice[]>([])
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [logs, setLogs] = useState('')
  const [serverCheck, setServerCheck] = useState('')
  const [confirmLeave, setConfirmLeave] = useState(false)

  const videoRef = useRef<HTMLDivElement>(null)
  const hideTimer = useRef<number>(0)
  const noticeId = useRef(0)

  const toast = useCallback((text: string, kind: Notice['kind'] = 'info') => {
    const id = ++noticeId.current
    setNotices((prev) => [...prev.slice(-3), { id, text, kind }])
    window.setTimeout(() => setNotices((prev) => prev.filter((n) => n.id !== id)), 4000)
  }, [])

  // 把视频区矩形同步给 Go，Go 据此摆放 mpv 宿主子窗口
  const syncRect = useCallback(() => {
    const el = videoRef.current
    if (!el || page !== 'player') return
    const r = el.getBoundingClientRect()
    SetVideoRect(r.left, r.top, r.width, r.height, window.innerWidth, window.innerHeight)
  }, [page])

  // 进入播放页才让 mpv 画面出现，回到首页则隐藏
  useEffect(() => {
    if (page === 'player') ShowVideoHost(true)
    else ShowVideoHost(false)
  }, [page])

  useEffect(() => {
    syncRect()
    const el = videoRef.current
    const ro = new ResizeObserver(() => syncRect())
    if (el) ro.observe(el)
    window.addEventListener('resize', syncRect)
    const t = window.setInterval(syncRect, 700)
    return () => {
      ro.disconnect()
      window.removeEventListener('resize', syncRect)
      window.clearInterval(t)
    }
  }, [syncRect])

  // 自动化联调入口：带环境变量启动时自动载片并进房，正常使用不会走到
  useEffect(() => {
    LogMsg('界面已挂载')
    DebugIntent().then(async (d: any) => {
      LogMsg('调试意图: ' + JSON.stringify(d))
      if (!d?.video) return
      const nm = d.name || '测试端'
      setName(nm)
      setServer((s) => s || '127.0.0.1:18998')
      try {
        const v: any = await LoadVideo(d.video)
        if (v) setVideo({ path: v.path, name: v.name, size: v.size })
        await SaveConfig({ server: '127.0.0.1:18998', name: nm, lastVideo: d.video } as any)
        if (d.room === 'new') {
          await CreateRoom(nm, '')
        } else if (d.room) {
          setRoomInput(d.room)
          await JoinRoom(d.room, nm, '')
        }
      } catch (e: any) {
        setError(String(e))
      }
    }).catch((e: any) => LogMsg('调试入口失败: ' + String(e)))
  }, [])

  useEffect(() => {
    GetConfig().then((c: any) => {
      if (c?.server) setServer(c.server)
      if (c?.name) setName(c.name)
    }).catch(() => { })
    EventsOn('host:error', (m: string) => setError(String(m)))
  }, [])

  // 房间与播放器事件
  useEffect(() => {
    const offs = [
      EventsOn('room:joined', (d: any) => {
        setRoom(d?.room || '')
        setConn('connected')
        setPage('player')
        setBusy(false)
        if (d?.created) {
          toast(`已创建房间 ${d.room}，把房间码发给朋友`)
        } else if (roomInput && d?.room && String(roomInput).toUpperCase() !== d.room) {
          setError(`你输入的房间码 ${String(roomInput).toUpperCase()} 不存在或已解散，本次创建了新房间 ${d.room}`)
        } else {
          toast(`已加入房间 ${d.room}`)
        }
        if (Array.isArray(d?.members)) setMembers(d.members)
      }),
      EventsOn('room:state', (d: any) => {
        if (d?.operator) {
          if (d.reason === 'forceSeek') {
            toast(`服务器已纠正你的播放进度到 ${fmtTime(d.position)}`, 'warn')
          } else {
            const verb = d.state === 'playing' ? '继续播放' : '暂停了'
            toast(`${d.operator} ${verb} · ${fmtTime(d.position)}`)
          }
        }
      }),
      EventsOn('room:members', (m: any) => {
        if (Array.isArray(m)) setMembers(m)
      }),
      EventsOn('room:notice', (t: string) => toast(String(t))),
      EventsOn('room:error', (e: string) => {
        setBusy(false)
        setError(String(e))
        setConn('disconnected')
      }),
      EventsOn('room:closed', () => {
        setConn('disconnected')
        toast('与服务器的连接已断开', 'error')
      }),
      EventsOn('player:loaded', (v: any) => {
        if (v?.path) setVideo({ path: v.path, name: v.name, size: v.size })
      }),
      EventsOn('player:paused', (p: boolean) => setPaused(Boolean(p))),
      EventsOn('player:exited', () => toast('播放器已退出', 'warn')),
    ]
    return () => offs.forEach((off) => off())
  }, [roomInput, toast])

  // 轮询状态：进度显示不依赖事件推送，避免漏事件导致时间不动
  useEffect(() => {
    if (page !== 'player') return
    const t = window.setInterval(async () => {
      try {
        const s: any = await GetState()
        if (!s) return
        if (!scrubbing) setPos(Number(s.position) || 0)
        if (Number(s.duration) > 0) setDur(Number(s.duration))
        setPaused(Boolean(s.paused))
        if (s.conn) setConn(s.conn)
        if (s.room) setRoom(s.room)
        if (Array.isArray(s.members)) setMembers(s.members)
      } catch { /* 播放器尚未就绪 */ }
    }, 1000)
    return () => window.clearInterval(t)
  }, [page, scrubbing])

  // 全屏：鼠标静止 2 秒后隐藏控制条
  useEffect(() => {
    if (!full) {
      setCtrlVisible(true)
      return
    }
    const wake = () => {
      setCtrlVisible(true)
      window.clearTimeout(hideTimer.current)
      hideTimer.current = window.setTimeout(() => setCtrlVisible(false), 2000)
    }
    wake()
    window.addEventListener('mousemove', wake)
    window.addEventListener('keydown', wake)
    return () => {
      window.removeEventListener('mousemove', wake)
      window.removeEventListener('keydown', wake)
      window.clearTimeout(hideTimer.current)
    }
  }, [full])

  const pickVideo = async () => {
    setError('')
    try {
      const r: any = await ChooseVideo()
      if (!r) return
      setVideo({ path: r.path, name: r.name, size: r.size })
    } catch (e: any) {
      setError(String(e))
    }
  }

  const doCreate = async () => {
    setError('')
    if (!video) { setError('请先选择一部本地视频'); return }
    if (!name.trim()) { setError('请填写昵称'); return }
    setBusy(true)
    try {
      await SaveConfig({ server, name, lastVideo: video.path } as any)
      await CreateRoom(name.trim(), password)
    } catch (e: any) {
      setBusy(false)
      setError(String(e))
    }
  }

  const doJoin = async () => {
    setError('')
    if (!roomInput.trim()) { setError('请填写房间码'); return }
    if (!video) { setError('请先选择一部本地视频'); return }
    if (!name.trim()) { setError('请填写昵称'); return }
    setBusy(true)
    try {
      await SaveConfig({ server, name, lastVideo: video.path } as any)
      await JoinRoom(roomInput.trim(), name.trim(), password)
    } catch (e: any) {
      setBusy(false)
      setError(String(e))
    }
  }

  const toggle = async () => {
    try {
      const p = await TogglePlay()
      setPaused(Boolean(p))
    } catch (e: any) { toast(String(e), 'error') }
  }

  const doSeek = async (v: number) => {
    setPos(v)
    try { await Seek(v) } catch (e: any) { toast(String(e), 'error') }
  }

  const changeRate = async (r: number) => {
    setRate(r)
    try { await SetRate(r) } catch (e: any) { toast(String(e), 'error') }
  }

  const changeVolume = async (v: number) => {
    setVolume(v)
    try { await SetVolume(v) } catch { /* 忽略 */ }
  }

  const toggleFull = async () => {
    const f = await ToggleFullscreen()
    setFull(Boolean(f))
    window.setTimeout(syncRect, 260)
  }

  const leave = async () => {
    setConfirmLeave(false)
    await LeaveRoom().catch(() => { })
    setPage('home')
    setRoom('')
    setMembers([])
    setConn('idle')
    setPos(0)
    setDur(0)
    setPaused(true)
    setCtrlVisible(true)
  }

  const openLogs = async () => {
    setLogs(await GetLogs().catch(() => ''))
    setPanel('logs')
  }

  const checkServer = async () => {
    setServerCheck('检测中…')
    try {
      const r: any = await PingServer(server)
      setServerCheck(r?.message || (r?.ok ? '连接正常' : '连接失败'))
    } catch (e: any) {
      setServerCheck(String(e))
    }
  }

  const otherDur = useMemo(() => {
    return members.some((m) => m.duration && dur > 0 && Math.abs(m.duration - dur) > 2)
  }, [members, dur])

  const noticeBar = (
    <div className="notices">
      {notices.map((n) => (
        <div key={n.id} className={`notice ${n.kind}`}>
          {n.kind === 'info' ? <IconCheck size={16} /> : <IconAlert size={16} />}
          <span>{n.text}</span>
        </div>
      ))}
    </div>
  )

  if (page === 'home') {
    return (
      <div className="app">
        <TopBar onSettings={() => setPanel('settings')} onLogs={openLogs} />
        <div className="home">
          <div className="hero">
            <h1>和朋友，看同一时刻。</h1>
            <p>本地视频不上传，只同步播放控制。</p>
          </div>

          <label className="field narrow">
            <span className="label">昵称</span>
            <input className="input" value={name} placeholder="房间里显示的名字"
              onChange={(e) => setName(e.target.value)} maxLength={16} />
          </label>

          <div className="entry-row">
            <button className="entry-card" onClick={() => { setError(''); setPage('create') }}>
              <IconPlus size={22} />
              <span className="entry-title">创建房间</span>
              <span className="entry-sub">选择视频并邀请朋友</span>
            </button>
            <button className="entry-card" onClick={() => { setError(''); setPage('join') }}>
              <IconArrowRight size={22} />
              <span className="entry-title">加入房间</span>
              <span className="entry-sub">输入朋友的房间码</span>
            </button>
          </div>

          <div className="server-line">
            <span className={server ? 'ok' : 'warn'}>
              {server ? `服务器 ${server}` : '未配置服务器地址'}
            </span>
            <button className="link" onClick={() => setPanel('settings')}>高级设置</button>
          </div>
          {error && <div className="error-line">{error}</div>}
        </div>
        {noticeBar}
        {panel === 'settings' && (
          <SettingsPanel
            server={server} setServer={setServer} name={name} setName={setName}
            check={serverCheck} onCheck={checkServer}
            onClose={() => setPanel(null)}
            onSave={async () => { await SaveConfig({ server, name } as any); setPanel(null); toast('设置已保存') }}
          />
        )}
        {panel === 'logs' && <LogsPanel logs={logs} onClose={() => setPanel(null)} onClear={async () => { await ClearLogs(); setLogs(''); toast('日志已清空') }} />}
      </div>
    )
  }

  if (page === 'create' || page === 'join') {
    const isJoin = page === 'join'
    return (
      <div className="app">
        <TopBar onSettings={() => setPanel('settings')} onLogs={openLogs} />
        <div className="form-page">
          <button className="back" onClick={() => { setPage('home'); setError('') }}>
            <IconBack size={18} /> 返回
          </button>
          <h2>{isJoin ? '加入房间' : '创建房间'}</h2>

          {isJoin && (
            <label className="field">
              <span className="label">房间码 <em>*</em></span>
              <input
                className="input code-input"
                value={roomInput}
                placeholder="例如 6ZJ46H"
                onChange={(e) => setRoomInput(e.target.value.toUpperCase().replace(/\s/g, ''))}
                maxLength={8}
              />
            </label>
          )}

          <div className="field">
            <span className="label">本地视频 <em>*</em></span>
            <button className={`dropzone ${video ? 'has-file' : ''}`} onClick={pickVideo}>
              {video ? (
                <>
                  <IconFilm size={20} />
                  <span className="file-name">{video.name}</span>
                  <span className="file-meta">{fmtSize(video.size)}</span>
                  <span className="file-change">重新选择</span>
                </>
              ) : (
                <>
                  <IconFilm size={22} />
                  <span>点击选择视频文件</span>
                  <span className="dim">支持 mpv 可播放的本地格式</span>
                </>
              )}
            </button>
          </div>

          <label className="field">
            <span className="label">昵称 <em>*</em></span>
            <input className="input" value={name} onChange={(e) => setName(e.target.value)} maxLength={16} />
          </label>

          <label className="field">
            <span className="label">房间密码 <span className="dim">（可选）</span></span>
            <input className="input" value={password} placeholder="不填则凭房间码即可进入"
              onChange={(e) => setPassword(e.target.value)} maxLength={16} />
          </label>

          {error && <div className="error-box"><IconAlert size={16} />{error}</div>}

          <div className="form-actions">
            <button className="btn primary lg" disabled={busy} onClick={isJoin ? doJoin : doCreate}>
              {busy ? '连接中…' : isJoin ? '加入并同步' : '创建并开始观看'}
            </button>
          </div>
        </div>
        {noticeBar}
        {panel === 'settings' && (
          <SettingsPanel
            server={server} setServer={setServer} name={name} setName={setName}
            check={serverCheck} onCheck={checkServer}
            onClose={() => setPanel(null)}
            onSave={async () => { await SaveConfig({ server, name } as any); setPanel(null); toast('设置已保存') }}
          />
        )}
        {panel === 'logs' && <LogsPanel logs={logs} onClose={() => setPanel(null)} onClear={async () => { await ClearLogs(); setLogs(''); toast('日志已清空') }} />}
      </div>
    )
  }

  const showPanel = panel === 'members'
  return (
    <div className={`app player ${full ? 'is-full' : ''} ${ctrlVisible ? '' : 'hide-ctrl'}`}>
      <div className={`stage ${showPanel ? 'with-panel' : ''}`}>
        <div className="video-wrap" ref={videoRef}>
          <div className="video-area" />
        </div>

        <div className="overlay top">
          <div className="title-line">
            <span className="film-name">{video?.name || '本地视频'}</span>
            <button className="room-chip" onClick={() => { CopyText(room); toast(`房间码 ${room} 已复制`) }}>
              <span className="mono">{room}</span>
              <IconCopy size={14} />
            </button>
            <span className={`conn-chip ${conn}`}>
              <i /> {conn === 'connected' ? '已连接' : conn === 'connecting' ? '连接中' : '已断开'}
            </span>
            <span className="dur-chip mono">{fmtTime(pos)} / {fmtTime(dur)}</span>
          </div>
          <div className="top-actions">
            <button className={`icon-btn ${otherDur ? 'has-warn' : ''}`}
              title={otherDur ? '有成员片源时长不一致' : '成员'}
              onClick={() => setPanel(showPanel ? null : 'members')}>
              <IconUsers size={18} />
              {otherDur && <i className="warn-dot" />}
            </button>
            <button className="icon-btn" title={full ? '退出全屏' : '全屏'} onClick={toggleFull}>
              {full ? <IconExitFullscreen size={18} /> : <IconFullscreen size={18} />}
            </button>
          </div>
        </div>

        {showPanel && (
          <aside className="members">
            <div className="members-head">
              <span>房间成员</span>
              <span className="count">{members.length}/5</span>
              <button className="icon-btn" onClick={() => setPanel(null)}><IconClose size={16} /></button>
            </div>
            <div className="members-body">
              {members.length === 0 && <div className="empty">还没有成员信息</div>}
              {members.map((m) => (
                <div className="member" key={m.name}>
                  <div className="member-row">
                    <span className={`dot ${stateDot(m.state)}`} />
                    <span className="member-name">{m.name}</span>
                  </div>
                  <div className="member-meta">
                    {stateCn(m.state)}{m.duration ? ` · ${fmtTime(m.duration)}` : ''}
                  </div>
                  {m.duration && dur > 0 && Math.abs(m.duration - dur) > 2 && (
                    <div className="member-warn"><IconAlert size={13} /> 时长与你不一致</div>
                  )}
                </div>
              ))}
            </div>
            {otherDur && <div className="members-tip">有成员片源时长不一致，可能越看越歪</div>}
          </aside>
        )}

        <div className="overlay bottom">
          {error && (
            <div className="blocking">
              <IconAlert size={18} />
              <span>{error}</span>
              <button className="btn sm" onClick={() => setError('')}>知道了</button>
            </div>
          )}
          <div className="ctrlbar">
            <button className="play-btn" onClick={toggle} title={paused ? '播放' : '暂停'}>
              {paused ? <IconPlay size={20} /> : <IconPause size={20} />}
            </button>
            <span className="time mono">{fmtTime(pos)}</span>
            <input
              className="scrub" type="range" min={0} max={dur > 0 ? dur : 1} step={0.2}
              value={Math.min(pos, dur || 1)}
              onChange={(e) => { setScrubbing(true); setPos(Number(e.target.value)) }}
              onMouseUp={() => { setScrubbing(false); doSeek(pos) }}
              onTouchEnd={() => { setScrubbing(false); doSeek(pos) }}
            />
            <span className="time mono dim">{fmtTime(dur)}</span>

            <div className="rate-group">
              {[0.5, 1, 1.5, 2].map((r) => (
                <button key={r} className={`chip ${rate === r ? 'on' : ''}`} onClick={() => changeRate(r)}>
                  {r}×
                </button>
              ))}
            </div>

            <div className="vol">
              <button className="icon-btn" title="静音" onClick={() => changeVolume(volume > 0 ? 0 : 100)}>
                {volume > 0 ? <IconVolume size={18} /> : <IconMute size={18} />}
              </button>
              <input className="vol-slider" type="range" min={0} max={100} value={volume}
                onChange={(e) => changeVolume(Number(e.target.value))} />
            </div>

            <button className="btn ghost sm" onClick={() => setConfirmLeave(true)}>离开</button>
          </div>
        </div>

      </div>

      {noticeBar}

      {confirmLeave && (
        <div className="modal-mask" onClick={() => setConfirmLeave(false)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <h3>离开房间？</h3>
            <p>只退出当前房间，不会删除本地视频。</p>
            <div className="modal-actions">
              <button className="btn" onClick={() => setConfirmLeave(false)}>继续看</button>
              <button className="btn danger" onClick={leave}>离开</button>
            </div>
          </div>
        </div>
      )}

      {panel === 'logs' && <LogsPanel logs={logs} onClose={() => setPanel(null)} onClear={async () => { await ClearLogs(); setLogs(''); toast('日志已清空') }} />}
    </div>
  )
}

// ---------- 通用组件 ----------

function TopBar({ onSettings, onLogs }: { onSettings: () => void; onLogs: () => void }) {
  return (
    <header className="topbar">
      <div className="brand"><span className="brand-dot" /> 一起看 · SyncWatch</div>
      <div className="topbar-actions">
        <button className="icon-btn" title="设置" onClick={onSettings}><IconSettings size={18} /></button>
        <button className="icon-btn" title="运行日志" onClick={onLogs}><IconLog size={18} /></button>
      </div>
    </header>
  )
}

function SettingsPanel(props: {
  server: string; setServer: (s: string) => void
  name: string; setName: (s: string) => void
  check: string; onCheck: () => void
  onClose: () => void; onSave: () => void
}) {
  return (
    <div className="drawer-mask" onClick={props.onClose}>
      <div className="drawer" onClick={(e) => e.stopPropagation()}>
        <div className="drawer-head">
          <span>设置</span>
          <button className="icon-btn" onClick={props.onClose}><IconClose size={16} /></button>
        </div>
        <div className="drawer-body">
          <label className="field">
            <span className="label">服务器地址</span>
            <input className="input mono" value={props.server} placeholder="例如 1.2.3.4:8995"
              onChange={(e) => props.setServer(e.target.value)} />
          </label>
          <div className="row-between">
            <span className="dim small">填你自己的同步服务器，端口默认 8995</span>
            <button className="btn sm" onClick={props.onCheck}>测试连接</button>
          </div>
          {props.check && <div className="hint">{props.check}</div>}

          <label className="field">
            <span className="label">默认昵称</span>
            <input className="input" value={props.name} onChange={(e) => props.setName(e.target.value)} maxLength={16} />
          </label>

          <div className="note">
            <IconLink size={15} />
            <span>播放器使用程序目录下的 <code>mpv\mpv.exe</code>；找不到时可用环境变量 <code>SYNCWATCH_MPV</code> 指定。</span>
          </div>
        </div>
        <div className="drawer-foot">
          <button className="btn primary" onClick={props.onSave}>保存</button>
        </div>
      </div>
    </div>
  )
}

function LogsPanel({ logs, onClose, onClear }: { logs: string; onClose: () => void; onClear: () => void }) {
  return (
    <div className="drawer-mask" onClick={onClose}>
      <div className="drawer wide" onClick={(e) => e.stopPropagation()}>
        <div className="drawer-head">
          <span>运行日志</span>
          <div className="row-gap">
            <button className="btn sm" onClick={() => navigator.clipboard?.writeText(logs)}>复制</button>
            <button className="btn sm" onClick={onClear}>清空</button>
            <button className="icon-btn" onClick={onClose}><IconClose size={16} /></button>
          </div>
        </div>
        <pre className="logs">{logs || '暂无日志'}</pre>
      </div>
    </div>
  )
}
