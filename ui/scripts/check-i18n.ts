import { pathToFileURL } from "node:url"
import { relative, resolve } from "node:path"
import { readdir, readFile } from "node:fs/promises"
import ts from "typescript"

const visibleAttributeNames = new Set(["alt", "aria-label", "description", "label", "placeholder", "title"])
const toastMethodNames = new Set(["error", "info", "message", "success", "warning"])
const allowedVisibleLiterals = new Set([
  "$ task test",
  "Apache-2.0",
  "build-and-test",
  "CI Runner",
  "e2b",
  "GitHub",
  "GitHub App",
  "ID",
  "owner/repo",
  "owner/repo or owner/*",
  "PID",
  "Qiniu",
  "Qiniu Runner",
  "self-hosted,e2b",
  "ubuntu-24.04",
])

type CheckOptions = {
  resourcePaths: [string, string]
  skipResources: boolean
  sourceRoot: string
  skipSource: boolean
}

async function main() {
  const options = parseArguments(process.argv.slice(2))
  const errors: string[] = []
  if (!options.skipResources) {
    const [en, zh] = await Promise.all([
      loadResource(options.resourcePaths[0], "en"),
      loadResource(options.resourcePaths[1], "zh"),
    ])
    errors.push(...compareResources(en, zh))
  }
  if (!options.skipSource) {
    errors.push(...await checkSourceTree(options.sourceRoot))
  }
  if (errors.length > 0) {
    throw new Error(errors.join("\n"))
  }
}

function parseArguments(arguments_: string[]): CheckOptions {
  const options: CheckOptions = {
    resourcePaths: ["src/locales/en.ts", "src/locales/zh.ts"],
    skipResources: false,
    sourceRoot: "src",
    skipSource: false,
  }
  for (let index = 0; index < arguments_.length; index += 1) {
    switch (arguments_[index]) {
      case "--resources":
        if (!arguments_[index + 1] || !arguments_[index + 2]) {
          throw new Error("--resources requires English and Chinese module paths")
        }
        options.resourcePaths = [arguments_[index + 1], arguments_[index + 2]]
        index += 2
        break
      case "--skip-source":
        options.skipSource = true
        break
      case "--skip-resources":
        options.skipResources = true
        break
      case "--source-root":
        if (!arguments_[index + 1]) {
          throw new Error("--source-root requires a directory path")
        }
        options.sourceRoot = arguments_[index + 1]
        index += 1
        break
      default:
        throw new Error(`unknown argument: ${arguments_[index]}`)
    }
  }
  return options
}

async function loadResource(path: string, exportName: "en" | "zh") {
  const module = await import(pathToFileURL(resolve(path)).href)
  const resource = module[exportName]
  if (!resource || typeof resource !== "object" || Array.isArray(resource)) {
    throw new Error(`${path} must export an object named ${exportName}`)
  }
  return resource as Record<string, unknown>
}

function compareResources(
  en: unknown,
  zh: unknown,
  path = "",
): string[] {
  const errors: string[] = []
  const enType = resourceValueType(en)
  const zhType = resourceValueType(zh)
  if (enType !== zhType) {
    return [`type mismatch at ${path}: en=${enType}, zh=${zhType}`]
  }
  if (enType === "object" && isRecord(en) && isRecord(zh)) {
    for (const key of Object.keys(en)) {
      const keyPath = path ? `${path}.${key}` : key
      if (!(key in zh)) {
        errors.push(`missing from zh: ${keyPath}`)
        continue
      }
      errors.push(...compareResources(en[key], zh[key], keyPath))
    }
    for (const key of Object.keys(zh)) {
      const keyPath = path ? `${path}.${key}` : key
      if (!(key in en)) errors.push(`missing from en: ${keyPath}`)
    }
    return errors
  }
  if (enType === "array" && Array.isArray(en) && Array.isArray(zh)) {
    if (en.length !== zh.length) {
      errors.push(`array length mismatch at ${path}: en=${en.length}, zh=${zh.length}`)
    }
    const sharedLength = Math.min(en.length, zh.length)
    for (let index = 0; index < sharedLength; index += 1) {
      errors.push(...compareResources(en[index], zh[index], `${path}[${index}]`))
    }
    return errors
  }
  if (enType === "string" && typeof en === "string" && typeof zh === "string") {
    if (!en.trim()) errors.push(`empty value in en: ${path}`)
    if (!zh.trim()) errors.push(`empty value in zh: ${path}`)
    const enVariables = interpolationVariables(en)
    const zhVariables = interpolationVariables(zh)
    if (enVariables.join("\n") !== zhVariables.join("\n")) {
      errors.push(`interpolation mismatch at ${path}: en=[${enVariables.join(", ")}], zh=[${zhVariables.join(", ")}]`)
    }
    return errors
  }
  errors.push(`unsupported resource value at ${path}: ${enType}`)
  return errors
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === "object" && !Array.isArray(value))
}

function resourceValueType(value: unknown) {
  if (Array.isArray(value)) return "array"
  if (isRecord(value)) return "object"
  return typeof value
}

function interpolationVariables(value: string) {
  return Array.from(value.matchAll(/{{\s*([^},\s]+)[^}]*}}/g), (match) => match[1])
    .filter((variable, index, variables) => variables.indexOf(variable) === index)
    .sort()
}

async function checkSourceTree(sourceRoot: string) {
  const absoluteRoot = resolve(sourceRoot)
  const files = await sourceFiles(absoluteRoot)
  const errors: string[] = []
  for (const file of files) {
    const source = await readFile(file, "utf8")
    const sourceFile = ts.createSourceFile(
      file,
      source,
      ts.ScriptTarget.Latest,
      true,
      file.endsWith(".tsx") ? ts.ScriptKind.TSX : ts.ScriptKind.TS,
    )
    const report = (node: ts.Node, kind: string, value: string) => {
      const normalized = normalizeVisibleLiteral(value)
      if (!isUntranslatedVisibleLiteral(normalized)) return
      const location = sourceFile.getLineAndCharacterOfPosition(node.getStart(sourceFile))
      errors.push(
        `${relative(absoluteRoot, file)}:${location.line + 1}:${location.character + 1}: ${kind} must use i18n: ${normalized}`,
      )
    }
    const visit = (node: ts.Node) => {
      if (ts.isJsxText(node)) {
        report(node, "user-visible JSX text", node.text)
      } else if (ts.isJsxExpression(node) && node.expression && isJsxChildExpression(node)) {
        for (const literal of expressionLiterals(node.expression, sourceFile)) {
          report(literal.node, "user-visible JSX text", literal.text)
        }
      } else if (ts.isJsxAttribute(node)) {
        const attributeName = ts.isIdentifier(node.name) ? node.name.text : node.name.getText(sourceFile)
        if (!visibleAttributeNames.has(attributeName)) {
          ts.forEachChild(node, visit)
          return
        }
        if (node.initializer && ts.isStringLiteral(node.initializer)) {
          report(node.initializer, `user-visible ${attributeName}`, node.initializer.text)
        } else if (node.initializer && ts.isJsxExpression(node.initializer) && node.initializer.expression) {
          for (const literal of expressionLiterals(node.initializer.expression, sourceFile)) {
            report(literal.node, `user-visible ${attributeName}`, literal.text)
          }
        }
      } else if (ts.isCallExpression(node) && isToastCall(node) && node.arguments[0]) {
        for (const literal of expressionLiterals(node.arguments[0], sourceFile)) {
          report(literal.node, "toast message", literal.text)
        }
      }
      ts.forEachChild(node, visit)
    }
    visit(sourceFile)
  }
  return errors
}

async function sourceFiles(directory: string): Promise<string[]> {
  const entries = await readdir(directory, { withFileTypes: true })
  const files = await Promise.all(entries.map(async (entry) => {
    const path = resolve(directory, entry.name)
    if (entry.isDirectory()) {
      return entry.name === "locales" || entry.name === "node_modules" ? [] : sourceFiles(path)
    }
    if (!entry.isFile() || !/\.tsx?$/.test(entry.name) || /(?:\.test|\.d)\.tsx?$/.test(entry.name)) return []
    return [path]
  }))
  return files.flat().sort()
}

function isJsxChildExpression(node: ts.JsxExpression) {
  return ts.isJsxElement(node.parent) || ts.isJsxFragment(node.parent)
}

function isToastCall(node: ts.CallExpression) {
  return ts.isPropertyAccessExpression(node.expression)
    && ts.isIdentifier(node.expression.expression)
    && node.expression.expression.text === "toast"
    && toastMethodNames.has(node.expression.name.text)
}

function expressionLiterals(expression: ts.Expression, sourceFile: ts.SourceFile): Array<{ node: ts.Node; text: string }> {
  if (ts.isCallExpression(expression) && isTranslationCall(expression)) return []
  if (ts.isStringLiteral(expression) || ts.isNoSubstitutionTemplateLiteral(expression)) {
    return [{ node: expression, text: expression.text }]
  }
  if (ts.isTemplateExpression(expression)) {
    const staticText = expression.head.text
      + expression.templateSpans.map((span) => span.literal.text).join("")
    return [{ node: expression, text: staticText }]
  }
  if (ts.isConditionalExpression(expression)) {
    return [
      ...expressionLiterals(expression.whenTrue, sourceFile),
      ...expressionLiterals(expression.whenFalse, sourceFile),
    ]
  }
  if (ts.isBinaryExpression(expression)) {
    const outputOperators = new Set([
      ts.SyntaxKind.AmpersandAmpersandToken,
      ts.SyntaxKind.BarBarToken,
      ts.SyntaxKind.QuestionQuestionToken,
    ])
    if (expression.operatorToken.kind === ts.SyntaxKind.PlusToken) {
      return [
        ...expressionLiterals(expression.left, sourceFile),
        ...expressionLiterals(expression.right, sourceFile),
      ]
    }
    if (outputOperators.has(expression.operatorToken.kind)) {
      return expressionLiterals(expression.right, sourceFile)
    }
  }
  if (
    ts.isParenthesizedExpression(expression)
    || ts.isAsExpression(expression)
    || ts.isNonNullExpression(expression)
    || ts.isTypeAssertionExpression(expression)
  ) {
    return expressionLiterals(expression.expression, sourceFile)
  }
  return []
}

function isTranslationCall(node: ts.CallExpression) {
  if (ts.isIdentifier(node.expression)) return node.expression.text === "t"
  return ts.isPropertyAccessExpression(node.expression)
    && node.expression.name.text === "t"
    && ts.isIdentifier(node.expression.expression)
    && (node.expression.expression.text === "i18n" || node.expression.expression.text === "appI18n")
}

function normalizeVisibleLiteral(value: string) {
  return value.replace(/\s+/g, " ").trim()
}

function isUntranslatedVisibleLiteral(value: string) {
  return /\p{L}/u.test(value) && !allowedVisibleLiterals.has(value)
}

main().catch((error) => {
  console.error(error instanceof Error ? error.message : error)
  process.exitCode = 1
})
