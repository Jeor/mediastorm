// Named pages reuse the alternate layout editor. IDs never change on rename.
function isCustomHomePage(id) {
  return /^page-[a-zA-Z0-9-]{1,80}$/.test(id);
}
function homePageDefinitions() {
  const global = currentSettings.homeShelves?.views || {};
  const profile = selectedUserId ? userSettings?.homeShelves?.views || {} : {};
  const result = { ...global };
  for (const [id, view] of Object.entries(profile)) {
    result[id] = {
      ...view,
      name: view.name || global[id]?.name,
      icon: view.icon || global[id]?.icon,
      mediaFilter: view.mediaFilter || global[id]?.mediaFilter,
    };
  }
  return result;
}
function customHomePageIds() {
  const views = homePageDefinitions();
  return Object.keys(views)
    .filter((id) => isCustomHomePage(id) && !views[id].deleted)
    .sort(
      (a, b) =>
        (views[a].name || "Home page").localeCompare(
          views[b].name || "Home page",
        ) || a.localeCompare(b),
    );
}
function homePageNavigationOptions(options) {
  const views = homePageDefinitions();
  return [
    ...(options || []).filter((opt) => !isCustomHomePage(opt.value)),
    ...customHomePageIds().map((id) => ({
      value: id,
      label: views[id].name || "Home page",
    })),
  ];
}
function createHomePage() {
  const owner = homeViewOwner();
  owner.homeShelves ||= {};
  owner.homeShelves.views ||= {};
  const uniqueId = crypto.randomUUID
    ? crypto.randomUUID()
    : Array.from(crypto.getRandomValues(new Uint8Array(16)), (byte) =>
        byte.toString(16).padStart(2, "0"),
      ).join("");
  const id = "page-" + uniqueId;
  owner.homeShelves.views[id] = {
    name: "New page",
    icon: "home-variant",
    mediaFilter: "all",
    mode: "inherit",
  };
  editingHomeView = id;
  renderSettings();
}
function setHomePageMetadata(field, value) {
  if (!isCustomHomePage(editingHomeView)) return;
  const owner = homeViewOwner();
  owner.homeShelves ||= {};
  owner.homeShelves.views ||= {};
  // Editing metadata on a global page preserves its effective layout.
  owner.homeShelves.views[editingHomeView] ||= JSON.parse(
    JSON.stringify(
      homePageDefinitions()[editingHomeView] || { mode: "inherit" },
    ),
  );
  if (field === "name") value = value.trim().slice(0, 80) || "Home page";
  if (field === "mediaFilter" && !["all", "movies", "shows"].includes(value))
    return;
  if (!["name", "icon", "mediaFilter"].includes(field)) return;
  owner.homeShelves.views[editingHomeView][field] = value;
  renderSettings();
}
function deleteHomePage() {
  if (!isCustomHomePage(editingHomeView) || !confirm("Delete this home page?"))
    return;
  const owner = homeViewOwner();
  owner.homeShelves ||= {};
  owner.homeShelves.views ||= {};
  if (selectedUserId && currentSettings.homeShelves?.views?.[editingHomeView]) {
    owner.homeShelves.views[editingHomeView] = {
      mode: "inherit",
      deleted: true,
    };
  } else delete owner.homeShelves.views[editingHomeView];
  if (owner.display?.navigationTabVisibility) {
    owner.display.navigationTabVisibility =
      owner.display.navigationTabVisibility.filter(
        (id) => id !== editingHomeView,
      );
  }
  editingHomeView = "all";
  renderSettings();
}
function renderHomePageMetadata() {
  if (!isCustomHomePage(editingHomeView)) return "";
  const view = homePageDefinitions()[editingHomeView] || {};
  const icons = [
    "home-variant",
    "movie-open",
    "television-classic",
    "star",
    "heart",
    "animation",
    "popcorn",
    "compass-outline",
  ];
  const iconLabels = [
    "Home",
    "Movie",
    "Television",
    "Star",
    "Heart",
    "Animation",
    "Popcorn",
    "Discover",
  ];
  return (
    '<div class="form-group"><label class="form-label" for="home-page-name">Page name</label>' +
    '<input class="form-input" id="home-page-name" maxlength="80" value="' +
    escapeHtml(view.name || "Home page").replace(/"/g, "&quot;") +
    '" onchange="setHomePageMetadata(\'name\',this.value)"></div>' +
    '<div class="form-group"><label class="form-label" for="home-page-icon">Icon</label><select class="form-select" id="home-page-icon" onchange="setHomePageMetadata(\'icon\',this.value)">' +
    icons
      .map(
        (icon, i) =>
          '<option value="' +
          icon +
          '"' +
          ((view.icon || "home-variant") === icon ? " selected" : "") +
          ">" +
          iconLabels[i] +
          "</option>",
      )
      .join("") +
    "</select></div>" +
    '<div class="form-group"><label class="form-label" for="home-page-filter">Content</label><select class="form-select" id="home-page-filter" onchange="setHomePageMetadata(\'mediaFilter\',this.value)">' +
    ["all", "movies", "shows"]
      .map(
        (filter, i) =>
          '<option value="' +
          filter +
          '"' +
          ((view.mediaFilter || "all") === filter ? " selected" : "") +
          ">" +
          ["All", "Movies only", "Shows only"][i] +
          "</option>",
      )
      .join("") +
    "</select></div>" +
    '<p class="form-hint">Enable this page in App Navigation to show it as a tab or left-menu item.</p>' +
    '<button type="button" class="btn btn-secondary" onclick="deleteHomePage()">Delete page</button>'
  );
}
// Alternate Home layouts share the existing shelf editor and save transaction.
let editingHomeView = "all";
function homeViewOwner() {
  return selectedUserId && userSettings ? userSettings : currentSettings;
}
function isAlternateHomeViewInherited() {
  return (
    homeViewOwner().homeShelves?.views?.[editingHomeView]?.mode !== "custom"
  );
}
function effectiveHomeView() {
  const globalHome = currentSettings.homeShelves || {};
  const profileHome = selectedUserId ? userSettings?.homeShelves || {} : {};
  const base = { ...globalHome, ...profileHome };
  const globalShelves = globalHome.shelves || [];
  const profileShelves = profileHome.shelves;
  base.shelves = profileShelves?.length
    ? [
        ...profileShelves.map((s) => ({
          ...globalShelves.find((g) => g.id === s.id),
          ...s,
        })),
        ...globalShelves.filter(
          (g) => !profileShelves.some((s) => s.id === g.id),
        ),
      ]
    : globalShelves;
  const override =
    profileHome.views?.[editingHomeView] ?? globalHome.views?.[editingHomeView];
  return editingHomeView !== "all" && override?.mode === "custom"
    ? { ...base, ...override }
    : base;
}
function editableAlternateHomeView() {
  const owner = homeViewOwner();
  owner.homeShelves ||= {};
  owner.homeShelves.views ||= {};
  let view = owner.homeShelves.views[editingHomeView];
  if (view?.mode !== "custom") {
    const effective = effectiveHomeView();
    view = {
      ...homePageDefinitions()[editingHomeView],
      mode: "custom",
      shelves: JSON.parse(JSON.stringify(effective.shelves || [])),
    };
    for (const field of [
      "mobileTopShelfMode",
      "mobileTopShelfSourceId",
      "tvTopShelfMode",
      "tvTopShelfSourceId",
    ]) {
      if (effective[field] !== undefined) view[field] = effective[field];
    }
    owner.homeShelves.views[editingHomeView] = view;
  }
  view.shelves ||= JSON.parse(
    JSON.stringify(effectiveHomeView().shelves || []),
  );
  return view;
}
function setEditingHomeView(value) {
  editingHomeView = value;
  renderSettings();
}
function setHomeViewMode(mode) {
  if (mode === "custom") editableAlternateHomeView();
  else {
    const owner = homeViewOwner();
    owner.homeShelves ||= {};
    owner.homeShelves.views ||= {};
    if (mode === "global") delete owner.homeShelves.views[editingHomeView];
    else {
      const { name, icon, mediaFilter } =
        homePageDefinitions()[editingHomeView] || {};
      owner.homeShelves.views[editingHomeView] = {
        name,
        icon,
        mediaFilter,
        mode: "inherit",
      };
    }
  }
  renderSettings();
}
function setHomeViewHero(field, value) {
  const view = editableAlternateHomeView();
  const platform = field === "tvTopShelfMode" ? "tv" : "mobile";
  view[field] = value === "default" || value === "disabled" ? value : "shelf";
  view[platform + "TopShelfSourceId"] =
    value === "default" || value === "disabled" ? "" : value;
  renderSettings();
}
function setHomeViewLayoutNumber(field, value) {
  const view = editableAlternateHomeView();
  if (value === "") delete view[field];
  else {
    const parsed = Number(value);
    if (!Number.isFinite(parsed)) return;
    view[field] =
      field === "itemCap"
        ? Math.max(1, Math.min(100, Math.round(parsed)))
        : Math.max(0.5, Math.min(1, parsed));
  }
  renderSettings();
}
function renderHomeViewControls() {
  const tabs = ["all", "movies", "shows", ...customHomePageIds()]
    .map(
      (view) =>
        '<button type="button" class="btn ' +
        (view === editingHomeView ? "btn-primary" : "btn-secondary") +
        '" onclick="setEditingHomeView(\'' +
        view +
        "')\">" +
        escapeHtml(
          { all: "All", movies: "Movies", shows: "Shows" }[view] ||
            homePageDefinitions()[view]?.name ||
            "Home page",
        ) +
        "</button>",
    )
    .join(" ");
  let content =
    '<div style="margin-bottom:16px">' +
    tabs +
    ' <button type="button" class="btn btn-secondary" onclick="createHomePage()">Add page</button>' +
    renderHomePageMetadata();
  if (editingHomeView !== "all") {
    const view = homeViewOwner().homeShelves?.views?.[editingHomeView];
    content +=
      '<p class="form-hint">' +
      (view?.mode === "custom"
        ? "Custom layout. Only matching titles appear."
        : view?.mode === "inherit"
          ? "Inheriting Home layout."
          : "Using inherited layout settings.") +
      "</p>";
    content +=
      '<button type="button" class="btn btn-secondary" onclick="setHomeViewMode(\'custom\')">Customize</button> <button type="button" class="btn btn-secondary" onclick="setHomeViewMode(\'inherit\')">Reset to Home</button>';
    if (selectedUserId)
      content +=
        ' <button type="button" class="btn btn-secondary" onclick="setHomeViewMode(\'global\')">Use Global View</button>';
    const effective = effectiveHomeView();
    for (const [label, field, min, max, step] of [
      ["Items per shelf", "itemCap", 1, 100, 1],
      ["TV shelf scale", "homeShelfScale", 0.5, 1, 0.05],
      ["TV hero scale", "homeHeroScale", 0.5, 1, 0.05],
    ]) {
      content +=
        '<div class="form-group" style="margin-top:12px"><label class="form-label" for="home-view-' +
        field +
        '">' +
        label +
        "</label>" +
        '<input class="form-input" id="home-view-' +
        field +
        '" type="number" min="' +
        min +
        '" max="' +
        max +
        '" step="' +
        step +
        '" placeholder="Inherit Home" value="' +
        escapeHtml(view?.[field] ?? "") +
        '" onchange="setHomeViewLayoutNumber(\'' +
        field +
        "',this.value)\"></div>";
    }
    for (const [label, prefix] of [
      ["TV", "tv"],
      ["Mobile", "mobile"],
    ]) {
      const mode = effective[prefix + "TopShelfMode"] || "default";
      const current =
        mode === "shelf" ? effective[prefix + "TopShelfSourceId"] : mode;
      const options = [
        { id: "default", name: "Default Top 10" },
        { id: "disabled", name: "Disabled" },
        ...(effective.shelves || []).filter((s) => s.enabled),
      ];
      content +=
        '<div class="form-group" style="margin-top:12px"><label class="form-label" for="home-view-' +
        prefix +
        '-hero">' +
        label +
        ' top shelf</label><select class="form-select" id="home-view-' +
        prefix +
        '-hero" onchange="setHomeViewHero(\'' +
        prefix +
        "TopShelfMode',this.value)\">" +
        options
          .map(
            (s) =>
              '<option value="' +
              escapeHtml(s.id) +
              '"' +
              (s.id === current ? " selected" : "") +
              ">" +
              escapeHtml(s.name) +
              "</option>",
          )
          .join("") +
        "</select></div>";
    }
  }
  return content + "</div>";
}
