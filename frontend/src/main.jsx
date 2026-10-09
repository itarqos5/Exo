import React from 'react'
import { createRoot } from 'react-dom/client'
import './styles.css'
import App from './App'

// If rendering ever throws, show the error instead of a blank window.
class ErrorBoundary extends React.Component {
  state = { error: null }
  static getDerivedStateFromError(error) {
    return { error }
  }
  render() {
    if (!this.state.error) return this.props.children
    return (
      <div className="app crash">
        <h1>Something went wrong</h1>
        <p>The scan itself is unaffected: result.txt is still written next to exo.exe.</p>
        <pre>{String(this.state.error?.message ?? this.state.error)}</pre>
        <button className="btn ghost" onClick={() => this.setState({ error: null })}>
          Try again
        </button>
      </div>
    )
  }
}

createRoot(document.getElementById('root')).render(
  <React.StrictMode>
    <ErrorBoundary>
      <App />
    </ErrorBoundary>
  </React.StrictMode>
)
