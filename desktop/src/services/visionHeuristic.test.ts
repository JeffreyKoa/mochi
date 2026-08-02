/**
 * 举物启发式单测（vitest 未接入时可手动：npx tsx src/services/visionHeuristic.test.ts）
 */
import { looksLikeObjectQuery } from './visionHeuristic'

const cases: Array<[string, boolean]> = [
  ['我手里拿的什么', true],
  ['这是什么', true],
  ['看看这个', true],
  ['今天天气不错', false],
  ['这个好', false],
  ['你看啥物', true],
  ['what is this', true],
]

let failed = 0
for (const [text, want] of cases) {
  const got = looksLikeObjectQuery(text)
  if (got !== want) {
    console.error('FAIL', JSON.stringify(text), 'want', want, 'got', got)
    failed++
  }
}
if (failed > 0) {
  process.exit(1)
}
console.log('visionHeuristic: ok', cases.length, 'cases')
