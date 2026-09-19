export const verticalSwipeEasing = 'cubic-bezier(.2, .78, .18, 1)'
export const verticalReboundEasing = 'cubic-bezier(.22, 1, .36, 1)'

export function resistVerticalSwipe(distance, height, hasTarget = true) {
  const span = Math.max(1, height) * (hasTarget ? .68 : .28)
  return Math.sign(distance) * span * (1 - Math.exp(-Math.abs(distance) / span))
}

export function shouldCommitVerticalSwipe(distance, velocity, height) {
  return Math.abs(distance) >= Math.min(320, height * .4) ||
    (Math.abs(distance) > 128 && Math.abs(velocity) > 1.1)
}

export function verticalSettleDuration(displacement, height, velocity = 0, commit = true) {
  if (!commit) return Math.max(210, Math.min(280, 220 + Math.abs(displacement) * .16))
  const remaining = Math.max(0, 1 - Math.min(1, Math.abs(displacement) / Math.max(1, height)))
  return Math.max(220, Math.min(340, 185 + remaining * 150 - Math.min(1.5, Math.abs(velocity)) * 34))
}
