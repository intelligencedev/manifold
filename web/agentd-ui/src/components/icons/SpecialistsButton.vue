<template>
  <!-- Icon inspired by Solar by 480 Design - https://creativecommons.org/licenses/by/4.0/ -->
  <svg xmlns="http://www.w3.org/2000/svg" width="1em" height="1em" viewBox="0 0 24 24">
    <g fill="currentColor">
      <!-- solid shadow behind, kept close -->
      <rect x="4.5" y="4.5" width="18" height="18" rx="5" opacity="0.35" />
      <!-- face with eye cut-outs -->
      <mask id="agentEyes">
        <rect x="1.5" y="1.5" width="18" height="18" rx="5" fill="#fff" />
        <ellipse cx="7.5" cy="10" rx="2.3" ry="3.4" fill="#000" />
        <ellipse cx="13.5" cy="10" rx="2.3" ry="3.4" fill="#000" />
      </mask>
      <rect x="1.5" y="1.5" width="18" height="18" rx="5" mask="url(#agentEyes)" />
    </g>
  </svg>
</template>
