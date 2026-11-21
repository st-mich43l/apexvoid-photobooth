import { useEffect, useState } from 'react'

const MAX_SHOTS = 4

function App() {
  const [countdown, setCountdown] = useState<number | null>(null)
  const [shots, setShots] = useState<number[]>([])
  const [session, setSession] = useState(1)

  const complete = shots.length === MAX_SHOTS
  const running = countdown !== null

  useEffect(() => {
    if (countdown === null) return
    const timer = window.setTimeout(() => {
      if (countdown > 1) {
        setCountdown(countdown - 1)
      } else {
        setShots(previous => previous.length < MAX_SHOTS ? [...previous, previous.length + 1] : previous)
        setCountdown(null)
      }
    }, 1000)
    return () => window.clearTimeout(timer)
  }, [countdown])

  function reset() {
    setCountdown(null)
    setShots([])
    setSession(value => value + 1)
  }

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="logo-row"><span className="logo">✦</span><div><b>APEXVOID</b><small>PHOTOBOOTH</small></div></div>
        <div className="sidebar-subtitle">Desktop preview · 0.1.0</div>

        <div className="side-section">SESSION</div>
        <div className="side-link selected"><span>◉</span> Photo session <span className="dot" /></div>
        <div className="side-link dim"><span>▦</span> Frame templates <span className="soon">SOON</span></div>
        <div className="side-link dim"><span>▤</span> Session gallery <span className="soon">SOON</span></div>

        <div className="side-section second">HARDWARE</div>
        <div className="side-link dim"><span>◈</span> Camera setup <span className="soon">SOON</span></div>
        <div className="side-link dim"><span>▣</span> Printer setup <span className="soon">SOON</span></div>

        <div className="sidebar-bottom">
          <span className="status-dot" /> Offline demo mode
          <p>No Enterprise connection. No camera connected.</p>
        </div>
      </aside>

      <main className="workspace">
        <header className="toolbar">
          <div><div className="eyebrow">WORKSPACE / SHOOTING</div><h1>Your photo moment <span>✦</span></h1></div>
          <div className="toolbar-right"><span className="chip">Windows preview</span><span className="avatar">AV</span></div>
        </header>

        <div className="content-grid">
          <section className="capture-section">
            <div className="section-title"><div><h2>Live studio</h2><p>Try the four-shot session flow</p></div><span className="tag">SIMULATED CAMERA</span></div>
            <div className="viewfinder" aria-live="polite">
              <div className="viewfinder-top"><span className="record-dot" /> DEMO PREVIEW <span className="viewfinder-tag">FRAME 01 / 04</span></div>
              <div className="camera-stage">
                {countdown !== null ? (
                  <div className="countdown"><strong>{countdown}</strong><span>Get ready to smile</span></div>
                ) : complete ? (
                  <div className="camera-message"><div className="camera-symbol">✦</div><h3>Nice shots!</h3><p>Your demo session is complete.</p></div>
                ) : (
                  <div className="camera-message"><div className="camera-symbol">◉</div><h3>Make a little magic</h3><p>Real camera preview arrives in the hardware phase.</p></div>
                )}
              </div>
              <div className="viewfinder-bottom"><span>1280 × 800 · UI DEMO</span><span>NO IMAGE DATA</span></div>
            </div>
            <div className="capture-actions">
              <button className="primary" disabled={running || complete} onClick={() => setCountdown(3)}>
                <span>◉</span> {running ? 'Counting down…' : complete ? 'Session complete' : 'Take demo photo'}
              </button>
              <button className="secondary" onClick={reset}>{complete ? 'New session' : 'Reset session'}</button>
            </div>
            <p className="disclaimer">Preview mode simulates photo events only. It does not access your camera or save personal photos.</p>
          </section>

          <section className="strip-section">
            <div className="section-title"><div><h2>Photo strip</h2><p>4 frames · Vertical layout</p></div><span className="strip-count">{shots.length}/{MAX_SHOTS}</span></div>
            <div className="strip-paper">
              <div className="strip-heading">apexvoid <span>✦</span></div>
              <div className="strip-frames">
                {[1,2,3,4].map(number => (
                  <div className={shots.includes(number) ? 'strip-frame filled' : 'strip-frame'} key={number}>
                    {shots.includes(number) ? <><span className="sample-shape">✦</span><small>DEMO SHOT {number}</small></> : <><span>+</span><small>FRAME {String(number).padStart(2,'0')}</small></>}
                  </div>
                ))}
              </div>
              <div className="strip-foot">make moments last <span>♡</span></div>
            </div>
            <div className="info-card"><b>Studio checklist</b><div><span>✓</span> Desktop UI ready</div><div><span>✓</span> Demo countdown ready</div><div className="pending"><span>○</span> Canon R50 adapter pending</div><div className="pending"><span>○</span> Printer adapter pending</div></div>
          </section>
        </div>

        <footer className="app-footer"><span>APEXVOID PHOTOBOOTH · DESKTOP MVP</span><span>Session #{String(session).padStart(3,'0')} · No cloud required</span></footer>
      </main>
    </div>
  )
}
export default App
