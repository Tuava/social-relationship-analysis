import { defineStore } from "pinia";
import { api } from "@/services/api";

export type SystemCapabilities = {
  graph: {
    depth: { min: number; max: number | null; default: number };
    build: { max_nodes: number | null; max_edges: number | null; max_events: number | null; default_unlimited: boolean };
    view: {
      min_nodes: number;
      default_nodes: number;
      max_nodes: number | null;
      min_distance: number;
      max_distance: number | null;
      expand_default: number;
      expand_max_nodes: number | null;
      render_presets: number[];
    };
  };
  lists: { min_page_size: number; default_page_size: number; max_page_size: number; page_sizes: number[] };
};

let capabilitiesRequest: Promise<void> | null = null;

export const useSystemCapabilitiesStore = defineStore("systemCapabilities", {
  state: () => ({
    data: null as SystemCapabilities | null,
    loading: false,
    loaded: false,
    error: "",
  }),
  actions: {
    async load(force = false) {
      if (capabilitiesRequest) return capabilitiesRequest;
      if (this.loaded && !force) return;
      this.loading = true;
      this.error = "";
      const store = this;
      capabilitiesRequest = api.get("/api/v1/system/capabilities")
        .then((response) => {
          store.data = response.data.data as SystemCapabilities;
          store.loaded = true;
        })
        .catch((error: any) => {
          store.error = error.response?.data?.error || "系统能力加载失败";
        })
        .finally(() => {
          store.loading = false;
          capabilitiesRequest = null;
        });
      return capabilitiesRequest;
    },
  },
});
