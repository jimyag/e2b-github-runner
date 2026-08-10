import { describe, expect, test } from "bun:test"

import i18n from "../i18n"
import { deploymentRequirementItems, landingChecklistItems } from "./landing-content-utils"
import { runnerLifecycleItems, runnerPolicyItems } from "./landing-lifecycle-preview-utils"

describe("landing translated collections", () => {
  test("keep stable IDs while labels change with language", () => {
    const en = i18n.getFixedT("en")
    const zh = i18n.getFixedT("zh")

    expect(landingChecklistItems(en).map((item) => item.id)).toEqual(landingChecklistItems(zh).map((item) => item.id))
    expect(deploymentRequirementItems(en).map((item) => item.id)).toEqual(deploymentRequirementItems(zh).map((item) => item.id))
    expect(runnerPolicyItems(en).map((item) => item.id)).toEqual(runnerPolicyItems(zh).map((item) => item.id))
    expect(runnerLifecycleItems(en).map((item) => item.id)).toEqual(runnerLifecycleItems(zh).map((item) => item.id))
    expect(landingChecklistItems(en).map((item) => item.label)).not.toEqual(landingChecklistItems(zh).map((item) => item.label))
  })
})
