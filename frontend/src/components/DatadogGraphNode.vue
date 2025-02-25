<template>

  <div
    :style="{
      ...data.style,
      width: '100%',
      height: '100%',
      boxSizing: 'border-box',
    }"
    class="node-container tool-node datadog-graph-node"
    @mouseenter="isHovered = true"
    @mouseleave="isHovered = false"
  >
    <div :style="data.labelStyle" class="node-label">
      {{ data.type }}
    </div>

    <Handle style="width:12px; height:12px" type="target" position="left" id="input" />

    <div class="graph-container" ref="graphContainer"></div>

    <Handle style="width:12px; height:12px" type="source" position="right" id="output"/>

    <NodeResizer
      :is-resizable="true"
      :color="'#666'"
      :handle-style="resizeHandleStyle"
      :line-style="resizeHandleStyle"
      :min-width="200"
      :min-height="120"
      :node-id="props.id"
      @resize="onResize"
    />
  </div>
</template>

<script setup>
import { ref, onMounted, watch, nextTick } from "vue";
import { Handle, useVueFlow } from "@vue-flow/core";
import { NodeResizer } from "@vue-flow/node-resizer";
import * as d3 from "d3";

const { getEdges, findNode } = useVueFlow();

const props = defineProps({
  id: {
    type: String,
    required: true,
  },
  data: {
    type: Object,
    required: true,
    default: () => ({
      type: "DatadogGraphNode",
      style: {
        width: "400px",
        height: "300px",
        backgroundColor: "#1e1e1e",
        border: "2px solid #fd7702",
      },
      inputs: {},
      outputs: {},
    }),
  },
});

const graphContainer = ref(null);
let svg = null;

async function run() {
  console.log("=== RUN DatadogGraphNode:", props.id, "===");

  await nextTick();

  if (!graphContainer.value) {
    console.error("[DatadogGraphNode] graphContainer ref is not set!");
    return;
  }

  // Find edges that point into this node
  const connectedTargetEdges = getEdges.value.filter(
    (edge) => edge.target === props.id
  );

  console.log(
    "[DatadogGraphNode] Edges pointing into this node:",
    connectedTargetEdges.map((e) => e.id)
  );

  if (connectedTargetEdges.length === 0) {
    console.warn("[DatadogGraphNode] No connected target edges found for node:", props.id);
    return;
  }

  // For simplicity, just use the FIRST edge:
  const edge = connectedTargetEdges[0];
  const sourceNode = findNode(edge.source);
  if (!sourceNode) {
    console.error("[DatadogGraphNode] Source node not found:", edge.source);
    return;
  }

  const maybeOutput = sourceNode.data?.outputs?.result?.output;
  if (!maybeOutput) {
    console.warn("[DatadogGraphNode] sourceNode has no 'output' data.");
    return;
  }

  console.log("[DatadogGraphNode] Raw output from source node:", maybeOutput);

  let seriesData;
  try {
    if (typeof maybeOutput === "string") {
      // It's a JSON string
      console.log("[DatadogGraphNode] The output is a string; parsing once...");
      seriesData = JSON.parse(maybeOutput);
    } else {
      // It's an object (possibly a proxy). Turn it into plain data
      console.log("[DatadogGraphNode] The output is an object; removing reactivity...");
      const str = JSON.stringify(maybeOutput);
      seriesData = JSON.parse(str);
    }
  } catch (err) {
    console.error("[DatadogGraphNode] Failed to parse node output:", err);
    return;
  }

  console.log("[DatadogGraphNode] Final parsed data:", seriesData);

  if (!seriesData.series || !Array.isArray(seriesData.series)) {
    console.error(
      "[DatadogGraphNode] No valid 'series' array found in source node output:",
      seriesData
    );
    return;
  }

  // Clear old chart
  d3.select(graphContainer.value).selectAll("*").remove();

  // Margin/size
  const margin = { top: 20, right: 20, bottom: 30, left: 40 };
  const width = graphContainer.value.clientWidth - margin.left - margin.right;
  const height = graphContainer.value.clientHeight - margin.top - margin.bottom;

  const x = d3.scaleTime().range([0, width]);
  const y = d3.scaleLinear().range([height, 0]);

  // Flatten all pointlists for domain
  const allPoints = seriesData.series.reduce((acc, s) => {
    if (Array.isArray(s.pointlist)) {
      return acc.concat(s.pointlist);
    }
    return acc;
  }, []);

  if (!allPoints.length) {
    console.warn("[DatadogGraphNode] No points found in any series:", seriesData.series);
    return;
  }

  const xDomain = d3.extent(allPoints, (d) => new Date(d[0]));
  const yDomain = d3.extent(allPoints, (d) => d[1]);

  x.domain(xDomain);
  y.domain(yDomain);

  // Create SVG root
  svg = d3
    .select(graphContainer.value)
    .append("svg")
    .attr("width", width + margin.left + margin.right)
    .attr("height", height + margin.top + margin.bottom)
    .append("g")
    .attr("transform", `translate(${margin.left},${margin.top})`);

  // X axis
  svg
    .append("g")
    .attr("transform", `translate(0,${height})`)
    .call(d3.axisBottom(x).ticks(5))
    .style("color", "#999");

  // Y axis
  svg
    .append("g")
    .call(d3.axisLeft(y).ticks(5))
    .style("color", "#999");

  // line generator
  const line = d3
    .line()
    .x((d) => x(new Date(d[0])))
    .y((d) => y(d[1]));

  // Draw each time series
  seriesData.series.forEach((s) => {
    if (!Array.isArray(s.pointlist) || !s.pointlist.length) {
      console.warn("[DatadogGraphNode] Skipping empty series:", s);
      return;
    }
    svg
      .append("path")
      .datum(s.pointlist)
      .attr("fill", "none")
      .attr("stroke", "#fd7702")
      .attr("stroke-width", 2)
      .attr("d", line);
  });
  console.log("[DatadogGraphNode] Rendered chart successfully!");
}

// Called on resize
const onResize = () => {
  console.log("[DatadogGraphNode] onResize triggered for node:", props.id);
  if (props.data.inputs?.result) {
    run();
  }
};

// Watch for data changes
watch(
  () => props.data.inputs,
  (newVal) => {
    console.log("[DatadogGraphNode] data.inputs changed:", newVal);
    if (newVal?.result) {
      nextTick(() => {
        run();
      });
    }
  },
  { deep: true }
);

onMounted(() => {
  console.log("[DatadogGraphNode] mounted:", props.id);
  if (props.data.inputs?.result) {
    nextTick(() => {
      run();
    });
  }

  // Provide a run() method if needed
  if (!props.data.run) {
    props.data.run = run;
  }
});
</script>

<style scoped>
.datadog-graph-node {
  padding: 10px;
  border-radius: 12px;
  background-color: #1e1e1e;
  color: #eee;
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
}

.node-label {
  color: #eee;
  font-size: 14px;
  text-align: center;
  margin-bottom: 8px;
}

.graph-container {
  width: 100%;
  height: calc(100% - 30px);
  overflow: hidden;
}

/* Use :deep for styling D3's appended elements */
:deep(svg) {
  width: 100%;
  height: 100%;
}

:deep(.domain),
:deep(.tick line) {
  stroke: #666;
}

:deep(.tick text) {
  fill: #999;
}
</style>
