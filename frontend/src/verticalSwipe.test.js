import test from 'node:test'
import assert from 'node:assert/strict'
import { resistVerticalSwipe, shouldCommitVerticalSwipe, verticalSettleDuration } from './verticalSwipe.js'

test('vertical resistance follows the finger symmetrically with increasing damping', () => {
  for (const height of [568, 844, 1080]) {
    assert.equal(resistVerticalSwipe(0, height), 0)
    let previous = 0
    let previousDelta = Infinity
    for (let distance = 20; distance <= 600; distance += 20) {
      const value = resistVerticalSwipe(distance, height)
      assert(value > previous && value < distance)
      assert(value - previous < previousDelta)
      assert.equal(resistVerticalSwipe(-distance, height), -value)
      assert(resistVerticalSwipe(distance, height, false) < value)
      previousDelta = value - previous
      previous = value
    }
  }
})

test('slow pulls require the viewport threshold; flicks still require a minimum distance', () => {
  for (const height of [568, 844, 1080]) {
    const threshold = Math.min(320, height * .4)
    for (const sign of [-1, 1]) {
      assert.equal(shouldCommitVerticalSwipe(sign * (threshold - 1), .5, height), false)
      assert.equal(shouldCommitVerticalSwipe(sign * threshold, .5, height), true)
      assert.equal(shouldCommitVerticalSwipe(sign * 128, 5, height), false)
      assert.equal(shouldCommitVerticalSwipe(sign * 129, 1.2, height), true)
    }
  }
})

test('release has bounded settling and a quicker rebound', () => {
  assert(verticalSettleDuration(200, 844, 1.5) < verticalSettleDuration(200, 844, 0))
  for (const distance of [0, 100, 300, 1000]) {
    const commit = verticalSettleDuration(distance, 844)
    const rebound = verticalSettleDuration(distance, 844, 0, false)
    assert(commit >= 220 && commit <= 340)
    assert(rebound >= 210 && rebound <= 280)
  }
})
