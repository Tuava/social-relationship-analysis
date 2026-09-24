import { defineStore } from "pinia";

export type RenderStrategy = { maxDistance: number; maxNodes: number; collapseBeyond: boolean };

export type GraphLayout = "cose" | "concentric" | "grid";

type SavedWorkspace = {
  updatedAt?: number;
  targetQQ?: string;
  depth?: number;
  layout?: GraphLayout;
  nodeTypes?: string[];
  relationTypes?: string[];
  minimumWeight?: number;
  targetNeighborsOnly?: boolean;
  showLabels?: boolean;
  wheelZoom?: boolean;
  zoom?: number;
  pan?: { x: number; y: number };
  selectedNodeKey?: string | null;
  selectedEdgeKey?: string | null;
  isolatedNodeKey?: string | null;
  isolatedNetworkId?: string | null;
  lastNetworkId?: string | null;
  viewportNetworkId?: string | null;
  filterPanelOpen?: boolean;
  inspectorOpen?: boolean;
  inspectorTab?: string;
  expandedNodeKeys?: string[];
  compareNodeKeys?: string[];
  viewMode?: "graph" | "table" | "communities" | "timeline";
  maxDistance?: number;
  maxNodes?: number;
  collapseBeyond?: boolean;
};

const STORAGE_KEY = "sra.graph-workspace.v1";

function readSaved(): SavedWorkspace {
  try {
    return JSON.parse(localStorage.getItem(STORAGE_KEY) || "{}");
  } catch {
    return {};
  }
}

export const useGraphWorkspaceStore = defineStore("graphWorkspace", {
  state: () => {
    const saved = readSaved();
    return {
      targetQQ: saved.targetQQ || "",
      maxDistance: Number.isFinite(Number(saved.maxDistance)) && Number(saved.maxDistance) > 0 ? Number(saved.maxDistance) : 2,
      maxNodes: Number.isFinite(Number(saved.maxNodes)) && Number(saved.maxNodes) > 0 ? Number(saved.maxNodes) : 300,
      collapseBeyond: saved.collapseBeyond !== false,
      depth: Number.isFinite(Number(saved.depth)) && Number(saved.depth) > 0 ? Number(saved.depth) : 2,
      layout: saved.layout || ("concentric" as GraphLayout),
      nodeTypes: Array.isArray(saved.nodeTypes) ? saved.nodeTypes : ([] as string[]),
      relationTypes: Array.isArray(saved.relationTypes) ? saved.relationTypes : ([] as string[]),
      minimumWeight: Math.max(1, Number(saved.minimumWeight) || 1),
      targetNeighborsOnly: Boolean(saved.targetNeighborsOnly),
      showLabels: saved.showLabels !== false,
      wheelZoom: Boolean(saved.wheelZoom),
      zoom: Number.isFinite(Number(saved.zoom)) ? Math.min(2, Math.max(0.1, Number(saved.zoom))) : 1,
      pan: saved.pan && Number.isFinite(saved.pan.x) && Number.isFinite(saved.pan.y) ? saved.pan : { x: 0, y: 0 },
      selectedNodeKey: saved.selectedNodeKey || null,
      selectedEdgeKey: saved.selectedEdgeKey || null,
      isolatedNodeKey: saved.isolatedNodeKey || null,
      isolatedNetworkId: saved.isolatedNetworkId || null,
      lastNetworkId: saved.lastNetworkId || null,
      viewportNetworkId: saved.viewportNetworkId || null,
      filterPanelOpen: Boolean(saved.filterPanelOpen),
      inspectorOpen: saved.inspectorOpen !== undefined ? Boolean(saved.inspectorOpen) : true,
      inspectorTab: saved.inspectorTab || "profile",
      expandedNodeKeys: Array.isArray(saved.expandedNodeKeys) ? saved.expandedNodeKeys : ([] as string[]),
      compareNodeKeys: Array.isArray(saved.compareNodeKeys) ? saved.compareNodeKeys : ([] as string[]),
      viewMode: saved.viewMode || ("graph" as const),
      graphStats: {
        hasGraph: false,
        visibleNodes: 0,
        visibleEdges: 0,
        loadedNodes: 0,
        loadedEdges: 0,
        scopeNodes: 0,
        scopeEdges: 0,
        totalNodes: 0,
        totalEdges: 0,
        partialDisplay: false,
        partialData: false,
      },
      graphBusy: false,
      buildRequest: 0,
    };
  },
  getters: {
    renderStrategy(state): RenderStrategy {
      return { maxDistance: state.maxDistance, maxNodes: state.maxNodes, collapseBeyond: state.collapseBeyond };
    },
  },
  actions: {
    persist() {
      localStorage.setItem(
        STORAGE_KEY,
        JSON.stringify({
          updatedAt: Date.now(),
          targetQQ: this.targetQQ,
          depth: this.depth,
          layout: this.layout,
          nodeTypes: this.nodeTypes,
          relationTypes: this.relationTypes,
          minimumWeight: this.minimumWeight,
          targetNeighborsOnly: this.targetNeighborsOnly,
          showLabels: this.showLabels,
          wheelZoom: this.wheelZoom,
          zoom: this.zoom,
          pan: this.pan,
          selectedNodeKey: this.selectedNodeKey,
          selectedEdgeKey: this.selectedEdgeKey,
          isolatedNodeKey: this.isolatedNodeKey,
          isolatedNetworkId: this.isolatedNetworkId,
          lastNetworkId: this.lastNetworkId,
          viewportNetworkId: this.viewportNetworkId,
          filterPanelOpen: this.filterPanelOpen,
          inspectorOpen: this.inspectorOpen,
          inspectorTab: this.inspectorTab,
          expandedNodeKeys: this.expandedNodeKeys,
          compareNodeKeys: this.compareNodeKeys,
          viewMode: this.viewMode,
          maxDistance: this.maxDistance,
          maxNodes: this.maxNodes,
          collapseBeyond: this.collapseBeyond,
        }),
      );
    },
    setViewport(zoom: number, pan: { x: number; y: number }) {
      this.zoom = Math.min(2, Math.max(0.1, zoom));
      this.pan = { x: pan.x, y: pan.y };
      this.persist();
    },
    requestBuild() {
      this.buildRequest += 1;
    },
  },
});
