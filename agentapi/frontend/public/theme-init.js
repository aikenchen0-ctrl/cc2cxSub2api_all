(function () {
  var theme = null
  try {
    theme = localStorage.getItem('theme')
    if (theme !== 'dark' && theme !== 'light') {
      var legacy = [
        [localStorage, 'agentapi_landing_theme'],
        [sessionStorage, 'agentapi_theme'],
      ]
      for (var i = 0; i < legacy.length; i += 1) {
        var storage = legacy[i][0]
        var value = storage.getItem(legacy[i][1])
        if (!theme && (value === 'dark' || value === 'light')) {
          theme = value
          localStorage.setItem('theme', value)
        }
        storage.removeItem(legacy[i][1])
      }
    }
  } catch (_) {
    theme = null
  }
  if (theme === 'dark' || (theme !== 'light' && window.matchMedia('(prefers-color-scheme: dark)').matches)) {
    document.documentElement.classList.add('dark')
    var meta = document.querySelector('meta[name="theme-color"]')
    if (meta) meta.setAttribute('content', '#0e0c09')
  }
})()
