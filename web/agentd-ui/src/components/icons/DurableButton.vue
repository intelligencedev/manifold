<template>
  <!-- Icon inspired by Solar by 480 Design - https://creativecommons.org/licenses/by/4.0/ -->
  <svg xmlns="http://www.w3.org/2000/svg" width="1em" height="1em" viewBox="0 0 24 24" fill="currentColor">
    <defs>
      <mask id="durableLoop">
        <!-- whole icon scaled up to fill the canvas like the other glyphs -->
        <g transform="translate(-1.8,-1.8) scale(1.15)">
          <!-- shield = visible (white in mask) -->
          <path d="M11.64 2.16a1.2 1.2 0 0 1 .72 0l7.1 2.37a1 1 0 0 1 .69.95v5.97c0 4.7-3.1 8.16-7.74 10.6a.9.9 0 0 1-.82 0C6.95 19.6 3.85 16.15 3.85 11.45V5.48a1 1 0 0 1 .69-.95l7.1-2.37Z" fill="#fff" />
          <!-- retry loop = cut out (black in mask) -->
          <g transform="translate(4.56,3.7) scale(0.62)">
            <path d="M12 2a10 10 0 1 0 9.54 12.97a1 1 0 0 0-1.91-.59A8 8 0 1 1 12 4a7.96 7.96 0 0 1 5.66 2.34L15.7 8.3a.5.5 0 0 0 .35.85H21.5A.5.5 0 0 0 22 8.65V3.21a.5.5 0 0 0-.85-.36L19.1 4.9A10 10 0 0 0 12 2" fill="#000" />
          </g>
        </g>
      </mask>
    </defs>
    <!-- single solid fill, same weight/color as the rest of the icon set -->
    <rect width="24" height="24" fill="currentColor" mask="url(#durableLoop)" />
  </svg>
</template>
