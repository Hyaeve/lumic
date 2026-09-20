// The feed is virtualized, so this copies only its currently rendered window.
// Frozen computed styles preserve layout outside the original route ancestry.
const frozenProperties = `
  display position top right bottom left z-index box-sizing width height min-width
  max-width min-height max-height margin padding border border-radius border-collapse
  box-shadow background color opacity visibility overflow overflow-x overflow-y
  font-family font-size font-weight font-style line-height letter-spacing white-space
  word-break overflow-wrap text-align text-transform text-decoration text-overflow
  vertical-align -webkit-line-clamp -webkit-box-orient -webkit-text-size-adjust
  flex flex-direction flex-wrap align-items align-content align-self justify-content
  justify-items justify-self gap row-gap column-gap grid-template-columns
  grid-template-rows grid-column grid-row grid-auto-flow grid-auto-rows
  transform transform-origin aspect-ratio object-fit object-position filter
  backdrop-filter -webkit-backdrop-filter mask -webkit-mask clip-path
  content-visibility contain isolation pointer-events float clear
`.trim().split(/\s+/)

export function capturePageSnapshot(element) {
  if (!element) return null
  const rect = element.getBoundingClientRect()
  const clone = element.cloneNode(true)
  const originals = [element, ...element.querySelectorAll('*')]
  const copies = [clone, ...clone.querySelectorAll('*')]
  originals.forEach((original, index) => {
    const copy = copies[index]
    const style = getComputedStyle(original)
    for (const property of frozenProperties) copy.style.setProperty(property, style.getPropertyValue(property))
    copy.removeAttribute('id')
    copy.removeAttribute('autofocus')
    copy.style.setProperty('animation', 'none', 'important')
    copy.style.setProperty('transition', 'none', 'important')
    if (copy.tagName === 'IMG') {
      copy.src = original.currentSrc || original.src
      copy.removeAttribute('srcset')
      copy.loading = 'eager'
    }
    if (copy.tagName === 'VIDEO') {
      copy.removeAttribute('autoplay')
      copy.muted = true
    }
  })
  Object.assign(clone.style, {
    position: 'absolute', left: `${rect.left}px`, top: `${rect.top}px`,
    width: `${rect.width}px`, height: `${rect.height}px`, margin: '0',
    transform: 'none', opacity: '1', pointerEvents: 'none'
  })
  // WebKit serializes logical insets after physical ones. Override both so
  // its copied inset-block-start cannot reset the saved viewport offset.
  for (const property of ['inset', 'inset-block', 'inset-inline', 'inset-block-start', 'inset-block-end', 'inset-inline-start', 'inset-inline-end']) clone.style.removeProperty(property)
  clone.style.setProperty('top', `${rect.top}px`, 'important')
  clone.style.setProperty('left', `${rect.left}px`, 'important')
  clone.inert = true
  clone.setAttribute('aria-hidden', 'true')
  return clone
}
