import assert from "node:assert/strict";
import {
    truncateText,
    extractTags,
    formatPublishedAt,
    createFilterParams,
    buildSubscriptionPayload,
    setActiveView,
    createPageRange,
} from "./app.js";

function test(name, fn) {
    try {
        fn();
        console.log(`ok - ${name}`);
    } catch (err) {
        console.error(`fail - ${name}`);
        console.error(err);
        process.exit(1);
    }
}

test("truncateText keeps short texts intact", () => {
    const input = "short text";
    const result = truncateText(input, 20);
    assert.equal(result.text, input);
    assert.equal(result.tooltip, input);
});

test("truncateText truncates long text and preserves tooltip", () => {
    const input =
        "this is a very long summary that should be truncated in the UI table for better readability";
    const result = truncateText(input, 30);
    assert.ok(result.text.endsWith("…"));
    assert.equal(result.tooltip, input);
    assert.ok(result.text.length <= 31);
});

test("extractTags returns sorted tag keys from JSON map", () => {
    const map = { Remote: true, PartTime: true };
    const tags = extractTags(map);
    assert.deepEqual(tags, ["PartTime", "Remote"]);
});

test("extractTags handles empty or invalid inputs", () => {
    assert.deepEqual(extractTags(null), []);
    assert.deepEqual(extractTags({}), []);
});

test("formatPublishedAt formats valid ISO time", () => {
    const text = formatPublishedAt("2024-06-11T08:00:00Z");
    assert.ok(text.includes("2024"));
});

test("formatPublishedAt handles bad input", () => {
    assert.equal(formatPublishedAt(""), "");
    assert.equal(formatPublishedAt("invalid"), "");
});

test("createFilterParams serializes filters", () => {
    const params = createFilterParams({
        tags: ["backend", "frontend"],
        employmentType: "full_time",
    });
    assert.ok(params.includes("tags=backend%2Cfrontend"));
    assert.ok(params.includes("employment_type=full_time"));
});

test("buildSubscriptionPayload trims and dedupes tags", () => {
    const payload = buildSubscriptionPayload({
        email: " test@example.com ",
        channel: "email",
        tags: ["Go", "GO", ""],
    });
    assert.equal(payload.email, "test@example.com");
    assert.deepEqual(payload.tags, ["Go"]);
});

function createStubSection() {
    return {
        hidden: false,
        setAttribute(name) {
            if (name === "hidden") {
                this.hidden = true;
            }
        },
        removeAttribute(name) {
            if (name === "hidden") {
                this.hidden = false;
            }
        },
    };
}

test("setActiveView hides inactive sections and returns selected id", () => {
    const jobSection = createStubSection();
    const subscriptionSection = createStubSection();
    const result = setActiveView("jobs", {
        jobs: jobSection,
        subscription: subscriptionSection,
    });
    assert.equal(result, "jobs");
    assert.equal(jobSection.hidden, false);
    assert.equal(subscriptionSection.hidden, true);
});

test("createPageRange returns continuous page numbers", () => {
    const pages = createPageRange(45, 20);
    assert.deepEqual(pages, [1, 2, 3]);
});

test("createPageRange falls back to single page when inputs invalid", () => {
    assert.deepEqual(createPageRange(0, 20), [1]);
    assert.deepEqual(createPageRange(10, 0), [1]);
    assert.deepEqual(createPageRange(-5, 20), [1]);
});
test("setActiveView falls back to first available section", () => {
    const jobSection = createStubSection();
    const subscriptionSection = createStubSection();
    const result = setActiveView("unknown", {
        jobs: jobSection,
        subscription: subscriptionSection,
    });
    assert.equal(result, "jobs");
    assert.equal(jobSection.hidden, false);
    assert.equal(subscriptionSection.hidden, true);
});
