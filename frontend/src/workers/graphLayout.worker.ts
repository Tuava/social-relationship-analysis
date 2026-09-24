type LayoutNode = { id: string; x: number; y: number; degree: number };
type LayoutEdge = { source: number; target: number; weight: number };
type LayoutRequest = { token: number; nodes: Array<{ id: string; degree: number }>; edges: Array<{ source: string; target: string; weight: number }> };

self.onmessage = (event: MessageEvent<LayoutRequest>) => {
  const { token, nodes: inputNodes, edges: inputEdges } = event.data;
  const count = inputNodes.length;
  if (!count) {
    self.postMessage({ token, positions: {} });
    return;
  }

  const radius = Math.max(260, Math.sqrt(count) * 42);
  const nodes: LayoutNode[] = inputNodes.map((node, index) => {
    const angle = (Math.PI * 2 * index) / count;
    const jitter = ((index * 2654435761) >>> 0) % 31;
    return {
      id: node.id,
      degree: Math.max(1, node.degree),
      x: Math.cos(angle) * (radius + jitter),
      y: Math.sin(angle) * (radius + jitter),
    };
  });
  const indices = new Map(nodes.map((node, index) => [node.id, index]));
  const edges: LayoutEdge[] = inputEdges.flatMap((edge) => {
    const source = indices.get(edge.source), target = indices.get(edge.target);
    return source === undefined || target === undefined ? [] : [{ source, target, weight: Math.max(1, edge.weight || 1) }];
  });
  const velocity = nodes.map(() => ({ x: 0, y: 0 }));
  const iterations = count > 1200 ? 90 : count > 600 ? 120 : 150;
  const cellSize = 150;

  for (let iteration = 0; iteration < iterations; iteration += 1) {
    const cells = new Map<string, number[]>();
    nodes.forEach((node, index) => {
      const key = `${Math.floor(node.x / cellSize)}:${Math.floor(node.y / cellSize)}`;
      const bucket = cells.get(key) || [];
      bucket.push(index);
      cells.set(key, bucket);
    });
    const force = nodes.map(() => ({ x: 0, y: 0 }));

    nodes.forEach((node, index) => {
      const cellX = Math.floor(node.x / cellSize), cellY = Math.floor(node.y / cellSize);
      for (let offsetX = -1; offsetX <= 1; offsetX += 1) {
        for (let offsetY = -1; offsetY <= 1; offsetY += 1) {
          for (const otherIndex of cells.get(`${cellX + offsetX}:${cellY + offsetY}`) || []) {
            if (otherIndex <= index) continue;
            const other = nodes[otherIndex];
            let dx = node.x - other.x, dy = node.y - other.y;
            const distanceSquared = Math.max(64, dx * dx + dy * dy);
            const distance = Math.sqrt(distanceSquared);
            const strength = 4200 / distanceSquared;
            dx = (dx / distance) * strength;
            dy = (dy / distance) * strength;
            force[index].x += dx; force[index].y += dy;
            force[otherIndex].x -= dx; force[otherIndex].y -= dy;
          }
        }
      }
    });

    for (const edge of edges) {
      const source = nodes[edge.source], target = nodes[edge.target];
      const dx = target.x - source.x, dy = target.y - source.y;
      const distance = Math.max(1, Math.sqrt(dx * dx + dy * dy));
      const ideal = 92 + Math.min(70, Math.log2(edge.weight + 1) * 8);
      const spring = (distance - ideal) * 0.0028;
      const fx = (dx / distance) * spring, fy = (dy / distance) * spring;
      force[edge.source].x += fx; force[edge.source].y += fy;
      force[edge.target].x -= fx; force[edge.target].y -= fy;
    }

    const cooling = 1 - iteration / iterations;
    nodes.forEach((node, index) => {
      force[index].x += -node.x * 0.00035;
      force[index].y += -node.y * 0.00035;
      velocity[index].x = (velocity[index].x + force[index].x) * 0.82;
      velocity[index].y = (velocity[index].y + force[index].y) * 0.82;
      const maximumStep = 18 * cooling + 2;
      node.x += Math.max(-maximumStep, Math.min(maximumStep, velocity[index].x));
      node.y += Math.max(-maximumStep, Math.min(maximumStep, velocity[index].y));
    });
  }

  // Expand dense force-layout results before handing them back to Cytoscape.
  // This keeps all nodes while avoiding a single unreadable center mass.
  const minX = Math.min(...nodes.map((node) => node.x));
  const maxX = Math.max(...nodes.map((node) => node.x));
  const minY = Math.min(...nodes.map((node) => node.y));
  const maxY = Math.max(...nodes.map((node) => node.y));
  const currentSpan = Math.max(1, maxX - minX, maxY - minY);
  const targetSpan = Math.max(900, Math.sqrt(count) * 88);
  const spread = Math.max(1, Math.min(3.5, targetSpan / currentSpan));
  const centerX = (minX + maxX) / 2;
  const centerY = (minY + maxY) / 2;
  const positions = Object.fromEntries(nodes.map((node) => [node.id, {
    x: (node.x - centerX) * spread,
    y: (node.y - centerY) * spread,
  }]));
  self.postMessage({ token, positions });
};

export {};
