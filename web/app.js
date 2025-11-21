const DEFAULT_SUMMARY_LIMIT = 120;
export const PAGE_SIZE = 20;

const state = {
    filters: { tags: [] },
    subscription: { tags: [] },
    meta: null,
};

const ACTIVE_NAV_CLASS = "nav-tab-active";

export function truncateText(text = "", maxLength = DEFAULT_SUMMARY_LIMIT) {
    const value = typeof text === "string" ? text.trim() : "";
    if (!value) {
        return { text: "", tooltip: "" };
    }
    if (value.length <= maxLength) {
        return { text: value, tooltip: value };
    }
    const cutoff = Math.max(0, maxLength - 1);
    const clipped = value.slice(0, cutoff).trimEnd();
    return { text: `${clipped}…`, tooltip: value };
}

export function extractTags(tagMap) {
    if (!tagMap || typeof tagMap !== "object") {
        return [];
    }
    const tags = [];
    for (const key of Object.keys(tagMap)) {
        if (tagMap[key]) {
            tags.push(key);
        }
    }
    return tags.sort((a, b) => a.localeCompare(b, "zh-CN"));
}

export function renderJobCard(job) {
    const card = document.createElement("article");
    card.className = "job-card";

    const header = document.createElement("div");
    header.className = "job-card-header";

    const title = document.createElement("h3");
    title.className = "job-card-title";
    title.textContent = job.title || "未命名";
    header.appendChild(title);

    const meta = document.createElement("div");
    meta.className = "job-card-meta";
    const published = formatPublishedAt(job.published_at);
    const publishedLabel = document.createElement("span");
    publishedLabel.textContent = published || "发布时间未知";
    const sourceLabel = document.createElement("span");
    sourceLabel.textContent = job.source || "未知来源";
    meta.appendChild(publishedLabel);
    meta.appendChild(sourceLabel);
    header.appendChild(meta);
    card.appendChild(header);

    const summaryBlock = document.createElement("p");
    summaryBlock.className = "job-card-summary";
    const raw = job.raw_attributes || {};
    const baseSummary =
        job.summary || raw.summary || raw.full_title || raw.excerpt || "";
    const summary = truncateText(baseSummary, 140);
    summaryBlock.textContent = summary.text || "暂无描述";
    summaryBlock.title = summary.tooltip || "";
    card.appendChild(summaryBlock);

    const tagList = extractTags(job.normalized_tags || job.tags);
    const tagsWrapper = document.createElement("div");
    tagsWrapper.className = "job-card-tags";
    if (tagList.length === 0) {
        const placeholder = document.createElement("span");
        placeholder.className = "tag tag-empty";
        placeholder.textContent = "未分类";
        tagsWrapper.appendChild(placeholder);
    } else {
        tagList.forEach((tag) => {
            const chip = document.createElement("span");
            chip.className = "tag";
            chip.textContent = tag;
            tagsWrapper.appendChild(chip);
        });
    }
    card.appendChild(tagsWrapper);

    const footer = document.createElement("div");
    footer.className = "job-card-footer";
    const linkLabel = document.createElement("span");
    linkLabel.className = "job-card-link-label";
    linkLabel.textContent = job.source || "官方链接";
    footer.appendChild(linkLabel);
    if (job.url) {
        const anchor = document.createElement("a");
        anchor.href = job.url;
        anchor.textContent = "查看详情";
        anchor.target = "_blank";
        anchor.rel = "noopener noreferrer";
        anchor.className = "job-card-link";
        footer.appendChild(anchor);
    } else {
        const missing = document.createElement("span");
        missing.className = "job-card-link missing";
        missing.textContent = "无链接";
        footer.appendChild(missing);
    }
    card.appendChild(footer);

    return card;
}

export function formatPublishedAt(input) {
    if (!input) {
        return "";
    }
    const date = new Date(input);
    if (Number.isNaN(date.getTime())) {
        return "";
    }
    return date.toLocaleString("zh-CN", { hour12: false });
}

export function createFilterParams(filters = {}) {
    const params = new URLSearchParams();
    if (filters.tags && filters.tags.length > 0) {
        params.set("tags", filters.tags.join(","));
    }
    if (filters.employmentType) {
        params.set("employment_type", filters.employmentType);
    }
    return params.toString();
}

export function buildSubscriptionPayload(input = {}) {
    const payload = {
        email: (input.email || "").trim(),
        channel: (input.channel || "email").trim() || "email",
        tags: [],
    };
    const seen = new Set();
    (input.tags || []).forEach((tag) => {
        const trimmed = String(tag || "").trim();
        if (!trimmed) {
            return;
        }
        const key = trimmed.toLowerCase();
        if (!seen.has(key)) {
            seen.add(key);
            payload.tags.push(trimmed);
        }
    });
    return payload;
}

function renderChannelOptions(selectEl, channels = []) {
    if (!selectEl) {
        return;
    }
    const seen = new Set();
    const normalized = channels
        .map((value) =>
            String(value || "")
                .trim()
                .toLowerCase(),
        )
        .filter(Boolean)
        .filter((value) => {
            if (seen.has(value)) {
                return false;
            }
            seen.add(value);
            return true;
        });
    const options = normalized.length > 0 ? normalized : ["email"];
    selectEl.innerHTML = "";
    options.forEach((channel) => {
        const option = document.createElement("option");
        option.value = channel;
        option.textContent = channel === "email" ? "邮箱" : channel;
        selectEl.appendChild(option);
    });
    selectEl.value = options[0];
}

export function setActiveView(activeId, sections = {}) {
    const keys = Object.keys(sections).filter((key) => Boolean(sections[key]));
    if (keys.length === 0) {
        return "";
    }
    const targetId = keys.includes(activeId) ? activeId : keys[0];
    keys.forEach((key) => {
        const section = sections[key];
        if (!section) {
            return;
        }
        if (key === targetId) {
            section.removeAttribute("hidden");
        } else {
            section.setAttribute("hidden", "hidden");
        }
    });
    return targetId;
}

export function createPageRange(totalItems, pageSize) {
    const total =
        Number.isFinite(totalItems) && totalItems > 0 ? totalItems : 0;
    const size =
        Number.isFinite(pageSize) && pageSize > 0 ? pageSize : PAGE_SIZE;
    const totalPages = Math.max(1, Math.ceil(total / size));
    return Array.from({ length: totalPages }, (_, index) => index + 1);
}

async function fetchMeta() {
    const res = await fetch("/api/meta");
    if (!res.ok) {
        throw new Error("加载筛选项失败");
    }
    return res.json();
}

export async function fetchJobs(page = 1, limit = PAGE_SIZE, filters = {}) {
    const params = new URLSearchParams({
        page: String(page),
        limit: String(limit),
    });
    const filterParams = createFilterParams(filters);
    if (filterParams) {
        const extra = new URLSearchParams(filterParams);
        for (const [key, value] of extra.entries()) {
            params.append(key, value);
        }
    }
    const res = await fetch(`/api/jobs?${params.toString()}`);
    if (!res.ok) {
        throw new Error(`fetch jobs failed: ${res.status}`);
    }
    const hasMoreHeader = res.headers.get("x-has-more");
    const totalHeader = res.headers.get("x-total");
    const total = Number(totalHeader || "0");
    const hasMore = hasMoreHeader === "true" || page * limit < total;
    const body = await res.json();
    return { jobs: body, hasMore, page, total };
}

export async function refreshJobs(container, options = {}) {
    const page = options.page ?? 1;
    const limit = options.limit ?? PAGE_SIZE;
    const filters = options.filters ?? {};
    const { jobs, hasMore, total } = await fetchJobs(page, limit, filters);
    if (!container) {
        return { jobs, hasMore, page, limit, total };
    }
    container.innerHTML = "";
    if (jobs.length === 0) {
        const empty = document.createElement("div");
        empty.className = "jobs-empty";
        empty.textContent = "暂时没有符合条件的职位";
        container.appendChild(empty);
    } else {
        jobs.forEach((job) => {
            container.appendChild(renderJobCard(job));
        });
    }
    return { jobs, hasMore, page, limit, total };
}

export async function handleManualRefresh(button, reloadFn) {
    button.disabled = true;
    button.textContent = "刷新中...";
    try {
        await fetch("/api/refresh", { method: "POST" });
        await reloadFn();
    } finally {
        button.disabled = false;
        button.textContent = "手动刷新";
    }
}

function renderTagSelector(
    container,
    tags = [],
    selected = [],
    onToggle = () => {},
) {
    if (!container) {
        return;
    }
    container.innerHTML = "";
    tags.forEach((tag) => {
        const label = document.createElement("label");
        label.className = "tag-option";
        const input = document.createElement("input");
        input.type = "checkbox";
        input.value = tag;
        input.checked = selected.includes(tag);
        input.addEventListener("change", () => onToggle(tag, input.checked));
        const span = document.createElement("span");
        span.textContent = tag;
        label.appendChild(input);
        label.appendChild(span);
        container.appendChild(label);
    });
}

function toggleTagSelection(list, value, enabled) {
    const next = Array.isArray(list) ? [...list] : [];
    const idx = next.indexOf(value);
    if (enabled && idx === -1) {
        next.push(value);
    }
    if (!enabled && idx >= 0) {
        next.splice(idx, 1);
    }
    return next;
}

async function submitSubscription(payload) {
    const res = await fetch("/api/subscriptions", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
    });
    if (!res.ok) {
        let message = "订阅失败";
        try {
            const body = await res.json();
            if (body.error) {
                message = body.error;
            }
        } catch (err) {
            message = err.message;
        }
        throw new Error(message);
    }
    return res.json();
}

function setSubscriptionStatus(el, message, isError) {
    if (!el) {
        return;
    }
    el.textContent = message;
    el.className = isError ? "status status-error" : "status status-success";
}

export function initJobsTable({
    jobsContainerId = "jobs",
    refreshBtnId = "refresh",
    filterContainerId = "filter-tags",
    applyFilterId = "apply-filters",
    subscriptionFormId = "subscription-form",
    subscriptionTagsId = "subscription-tags",
    subscriptionEmailId = "subscription-email",
    subscriptionChannelId = "subscription-channel",
    subscriptionStatusId = "subscription-status",
    pageSelectId = "page-select",
} = {}) {
    const jobsContainer = document.getElementById(jobsContainerId);
    const refreshButton = document.getElementById(refreshBtnId);
    const prevBtn = document.getElementById("prev-page");
    const nextBtn = document.getElementById("next-page");
    const indicator = document.getElementById("page-indicator");
    const totalIndicator = document.getElementById("total-indicator");
    const pageSelect = document.getElementById(pageSelectId);
    const filterContainer = document.getElementById(filterContainerId);
    const applyFilterBtn = document.getElementById(applyFilterId);
    const subscriptionForm = document.getElementById(subscriptionFormId);
    const subscriptionTags = document.getElementById(subscriptionTagsId);
    const subscriptionEmail = document.getElementById(subscriptionEmailId);
    const subscriptionChannel = document.getElementById(subscriptionChannelId);
    const subscriptionStatus = document.getElementById(subscriptionStatusId);
    const viewButtons = Array.from(
        document.querySelectorAll("[data-view-target]"),
    );
    const navLinkButtons = viewButtons.filter((button) =>
        button.hasAttribute("data-nav-link"),
    );
    const required = [
        jobsContainer,
        prevBtn,
        nextBtn,
        indicator,
        totalIndicator,
        pageSelect,
    ];
    if (required.some((el) => !el)) {
        console.warn("job elements not found, skip init");
        return;
    }

    const localState = { page: 1, limit: PAGE_SIZE, hasMore: false, total: 0 };

    function updatePaginationControls() {
        prevBtn.disabled = localState.page <= 1;
        const totalPages = Math.max(
            1,
            Math.ceil(localState.total / localState.limit),
        );
        nextBtn.disabled = localState.page >= totalPages;
        indicator.textContent = `第 ${localState.page} / ${totalPages} 页`;
        totalIndicator.textContent = `共 ${localState.total} 条职位`;
        const pages = createPageRange(localState.total, localState.limit);
        const currentValues = Array.from(pageSelect.options).map((option) =>
            Number(option.value),
        );
        const needUpdate =
            pages.length !== currentValues.length ||
            pages.some((value, index) => value !== currentValues[index]);
        if (needUpdate) {
            pageSelect.innerHTML = "";
            pages.forEach((pageNumber) => {
                const option = document.createElement("option");
                option.value = String(pageNumber);
                option.textContent = String(pageNumber);
                pageSelect.appendChild(option);
            });
        }
        pageSelect.value = String(localState.page);
    }

    async function loadPage(targetPage) {
        const { hasMore, total } = await refreshJobs(jobsContainer, {
            page: targetPage,
            limit: localState.limit,
            filters: state.filters,
        });
        localState.page = targetPage;
        localState.hasMore = hasMore;
        localState.total = total;
        updatePaginationControls();
    }

    if (refreshButton) {
        refreshButton.addEventListener("click", () =>
            handleManualRefresh(refreshButton, () =>
                loadPage(localState.page),
            ).catch((err) => console.error(err)),
        );
    } else {
        console.warn("refresh button missing; manual refresh disabled");
    }

    prevBtn.addEventListener("click", () => {
        if (localState.page > 1) {
            loadPage(localState.page - 1).catch((err) => console.error(err));
        }
    });

    nextBtn.addEventListener("click", () => {
        const totalPages = Math.max(
            1,
            Math.ceil(localState.total / localState.limit),
        );
        if (localState.page < totalPages) {
            loadPage(localState.page + 1).catch((err) => console.error(err));
        }
    });

    if (applyFilterBtn) {
        applyFilterBtn.addEventListener("click", () => {
            localState.page = 1;
            loadPage(1).catch((err) => console.error(err));
        });
    }

    pageSelect.addEventListener("change", () => {
        const target = Number(pageSelect.value);
        if (Number.isNaN(target) || target <= 0) {
            return;
        }
        loadPage(target).catch((err) => console.error(err));
    });

    if (subscriptionForm && subscriptionEmail && subscriptionChannel) {
        subscriptionForm.addEventListener("submit", (event) => {
            event.preventDefault();
            const payload = buildSubscriptionPayload({
                email: subscriptionEmail.value,
                channel: subscriptionChannel.value,
                tags: state.subscription.tags,
            });
            submitSubscription(payload)
                .then(() => {
                    setSubscriptionStatus(
                        subscriptionStatus,
                        "订阅成功，请注意查收",
                        false,
                    );
                })
                .catch((err) => {
                    setSubscriptionStatus(
                        subscriptionStatus,
                        err.message,
                        true,
                    );
                });
        });
    }

    if (viewButtons.length > 0) {
        const sections = viewButtons.reduce((acc, button) => {
            const targetView = button.dataset.viewTarget;
            if (targetView && !acc[targetView]) {
                acc[targetView] = document.getElementById(targetView);
            }
            return acc;
        }, {});
        const updateNavigation = (targetId) => {
            const activeViewId = setActiveView(targetId, sections);
            navLinkButtons.forEach((button) => {
                if (button.dataset.viewTarget === activeViewId) {
                    button.classList.add(ACTIVE_NAV_CLASS);
                    button.setAttribute("aria-current", "page");
                } else {
                    button.classList.remove(ACTIVE_NAV_CLASS);
                    button.removeAttribute("aria-current");
                }
            });
        };
        updateNavigation("jobs-view");
        viewButtons.forEach((button) => {
            button.addEventListener("click", () => {
                updateNavigation(button.dataset.viewTarget);
            });
        });
    }

    fetchMeta()
        .then((meta) => {
            state.meta = meta;
            const tagCandidates = meta.tag_candidates || [];
            renderChannelOptions(
                subscriptionChannel,
                Array.isArray(meta.channels) ? meta.channels : [],
            );
            renderTagSelector(
                filterContainer,
                tagCandidates,
                state.filters.tags,
                (tag, enabled) => {
                    state.filters.tags = toggleTagSelection(
                        state.filters.tags,
                        tag,
                        enabled,
                    );
                },
            );
            renderTagSelector(
                subscriptionTags,
                tagCandidates,
                state.subscription.tags,
                (tag, enabled) => {
                    state.subscription.tags = toggleTagSelection(
                        state.subscription.tags,
                        tag,
                        enabled,
                    );
                },
            );
        })
        .catch((err) => console.error(err))
        .finally(() => {
            loadPage(localState.page).catch((err) => console.error(err));
        });
}

if (typeof window !== "undefined") {
    window.addEventListener("DOMContentLoaded", () => {
        initJobsTable();
    });
}
