import { describe, expect, it } from 'vitest'
import { looksLikeObjectQuery } from './visionHeuristic'

describe('visionHeuristic', () => {
  const cases: Array<[string, boolean]> = [
    ['我手里拿的什么', true],
    ['这是什么', true],
    ['看看这个', true],
    ['今天天气不错', false],
    ['这个好', false],
    ['你看啥物', true],
    ['what is this', true],
  ]

  it.each(cases)('looksLikeObjectQuery(%j) => %s', (text, want) => {
    expect(looksLikeObjectQuery(text)).toBe(want)
  })
})
