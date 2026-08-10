import { afterEach, describe, expect, test } from "bun:test"
import { mkdtemp, rm, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"

const checkerPath = join(import.meta.dir, "check-i18n.ts")
const temporaryDirectories: string[] = []

afterEach(async () => {
  await Promise.all(temporaryDirectories.splice(0).map((directory) => rm(directory, { force: true, recursive: true })))
})

describe("i18n resource check", () => {
  test("accepts matching locale structures, arrays, and interpolation variables", async () => {
    const directory = await createFixture({
      en: {
        common: {
          greeting: "Hello {{name}}",
          requirements: ["GitHub.com", "Sandbox credentials"],
        },
      },
      zh: {
        common: {
          greeting: "你好，{{name}}",
          requirements: ["GitHub.com", "Sandbox 凭据"],
        },
      },
    })

    const result = await runChecker([
      "--resources",
      join(directory, "en.ts"),
      join(directory, "zh.ts"),
      "--skip-source",
    ])

    expect(result.exitCode, result.stderr).toBe(0)
  })

  test("rejects a key that is missing from the Chinese resource", async () => {
    const directory = await createFixture({
      en: { common: { save: "Save", cancel: "Cancel" } },
      zh: { common: { save: "保存" } },
    })

    const result = await runChecker([
      "--resources",
      join(directory, "en.ts"),
      join(directory, "zh.ts"),
      "--skip-source",
    ])

    expect(result.exitCode).toBe(1)
    expect(result.stderr).toContain("missing from zh: common.cancel")
  })

  test.each([
    {
      name: "different value types",
      en: { common: { label: "Label" } },
      zh: { common: { label: ["标签"] } },
      message: "type mismatch at common.label: en=string, zh=array",
    },
    {
      name: "an empty translation",
      en: { common: { label: "" } },
      zh: { common: { label: "标签" } },
      message: "empty value in en: common.label",
    },
    {
      name: "different array lengths",
      en: { common: { requirements: ["GitHub.com", "Sandbox credentials"] } },
      zh: { common: { requirements: ["GitHub.com"] } },
      message: "array length mismatch at common.requirements: en=2, zh=1",
    },
    {
      name: "an empty array entry",
      en: { common: { requirements: ["GitHub.com", ""] } },
      zh: { common: { requirements: ["GitHub.com", "Sandbox 凭据"] } },
      message: "empty value in en: common.requirements[1]",
    },
    {
      name: "different interpolation variables",
      en: { common: { greeting: "Hello {{name}}" } },
      zh: { common: { greeting: "你好，{{user}}" } },
      message: "interpolation mismatch at common.greeting: en=[name], zh=[user]",
    },
  ])("rejects $name", async ({ en, zh, message }) => {
    const directory = await createFixture({ en, zh })

    const result = await runChecker([
      "--resources",
      join(directory, "en.ts"),
      join(directory, "zh.ts"),
      "--skip-source",
    ])

    expect(result.exitCode).toBe(1)
    expect(result.stderr).toContain(message)
  })
})

describe("user-visible source check", () => {
  test("accepts translated copy and allowlisted technical literals", async () => {
    const directory = await createSourceFixture(`
      export function View({ t }: { t: (key: string) => string }) {
        return (
          <main>
            <h1>{t("common.title")}</h1>
            <span>Qiniu Runner</span>
            <input placeholder="owner/repo" />
          </main>
        )
      }
    `)

    const result = await runChecker([
      "--skip-resources",
      "--source-root",
      directory,
    ])

    expect(result.exitCode, result.stderr).toBe(0)
  })

  test("rejects untranslated JSX text, visible attributes, and toast messages", async () => {
    const directory = await createSourceFixture(`
      import { toast } from "sonner"

      export function View() {
        toast.error("Failed to start runner")
        return <button aria-label="Retry runner">Run job</button>
      }
    `)

    const result = await runChecker([
      "--skip-resources",
      "--source-root",
      directory,
    ])

    expect(result.exitCode).toBe(1)
    expect(result.stderr).toContain("toast message must use i18n: Failed to start runner")
    expect(result.stderr).toContain("user-visible aria-label must use i18n: Retry runner")
    expect(result.stderr).toContain("user-visible JSX text must use i18n: Run job")
  })
})

async function createFixture(resources: { en: unknown; zh: unknown }) {
  const directory = await mkdtemp(join(tmpdir(), "runnerd-i18n-"))
  temporaryDirectories.push(directory)
  await Promise.all([
    writeFile(join(directory, "en.ts"), `export const en = ${JSON.stringify(resources.en)} as const\n`),
    writeFile(join(directory, "zh.ts"), `export const zh = ${JSON.stringify(resources.zh)} as const\n`),
  ])
  return directory
}

async function createSourceFixture(source: string) {
  const directory = await mkdtemp(join(tmpdir(), "runnerd-i18n-source-"))
  temporaryDirectories.push(directory)
  await writeFile(join(directory, "view.tsx"), source)
  return directory
}

async function runChecker(arguments_: string[]) {
  const child = Bun.spawn([process.execPath, checkerPath, ...arguments_], {
    cwd: join(import.meta.dir, ".."),
    stderr: "pipe",
    stdout: "pipe",
  })
  const [exitCode, stderr] = await Promise.all([
    child.exited,
    new Response(child.stderr).text(),
  ])
  return { exitCode, stderr }
}
