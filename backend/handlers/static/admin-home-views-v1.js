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
    else owner.homeShelves.views[editingHomeView] = { mode: "inherit" };
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
  const tabs = ["all", "movies", "shows"]
    .map(
      (view) =>
        '<button type="button" class="btn ' +
        (view === editingHomeView ? "btn-primary" : "btn-secondary") +
        '" onclick="setEditingHomeView(\'' +
        view +
        "')\">" +
        { all: "All", movies: "Movies", shows: "Shows" }[view] +
        "</button>",
    )
    .join(" ");
  let content = '<div style="margin-bottom:16px">' + tabs;
  if (editingHomeView !== "all") {
    const view = homeViewOwner().homeShelves?.views?.[editingHomeView];
    content +=
      '<p class="form-hint">' +
      (view?.mode === "custom"
        ? "Custom layout. Only matching titles appear."
        : view?.mode === "inherit"
          ? "Inheriting this profile’s Home layout."
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
