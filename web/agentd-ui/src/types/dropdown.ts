/**
 * Option interface for dropdown components
 */
export interface DropdownOption {
  /** Unique identifier for the option */
  id: string | number;
  /** Display label for the option */
  label: string;
  /** Optional description or subtitle */
  description?: string;
  /** Whether this option is disabled */
  disabled?: boolean;
  /** Optional value that differs from id */
  value?: any;
}

/**
 * Size variants for dropdown components
 */
export type DropdownSize = "xs" | "sm" | "md" | "lg";

/**
 * Props interface for DropdownSelect component
 */
export interface DropdownSelectProps {
  /** Array of options to display */
  options: DropdownOption[];
  /** Currently selected value */
  modelValue?: any;
  /** Placeholder text when no option is selected */
  placeholder?: string;
  /** Size variant */
  size?: DropdownSize;
  /** Whether the dropdown is disabled */
  disabled?: boolean;
  /** Whether the dropdown is required */
  required?: boolean;
  /** Additional CSS classes */
  class?: string;
  /** HTML id attribute */
  id?: string;
  /** ARIA label for accessibility */
  ariaLabel?: string;
  /** Title attribute for tooltip */
  title?: string;
}
