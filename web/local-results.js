const prefix = 'orbital-run:local-result:'
const pending = new Map()

export function saveLocalResult(result) {
  pending.set(result.id, result)
  try {
    localStorage.setItem(prefix + result.id, JSON.stringify(result))
    return true
  } catch {
    return false
  }
}

export function pendingLocalResults() {
  const results = new Map(pending)
  try {
    for (let i = 0; i < localStorage.length; i++) {
      const key = localStorage.key(i)
      if (!key?.startsWith(prefix)) continue
      try {
        const result = JSON.parse(localStorage.getItem(key))
        if (result?.id === key.slice(prefix.length)) results.set(result.id, result)
      } catch {
        // A damaged entry should not prevent other results from syncing.
      }
    }
  } catch {
    // Keep sending results from this tab if browser storage is unavailable.
  }
  return results.values()
}

export function forgetLocalResult(id) {
  pending.delete(id)
  try {
    localStorage.removeItem(prefix + id)
  } catch {
    // If the browser retries a stored result later, the server deduplicates it.
  }
}
