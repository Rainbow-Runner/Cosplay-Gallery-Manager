import {
  forwardRef,
  type
  ButtonHTMLAttributes,
  InputHTMLAttributes,
  ReactNode,
  SelectHTMLAttributes,
  TextareaHTMLAttributes,
} from "react";

type ButtonVariant = "primary" | "secondary" | "ghost" | "danger";

function classes(...values: Array<string | false | undefined>) {
  return values.filter(Boolean).join(" ");
}

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
}

export function Button({ variant = "secondary", className, type = "button", ...props }: ButtonProps) {
  return <button {...props} type={type} className={classes("cgm-button", `cgm-button--${variant}`, className)} />;
}

interface IconButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  label: string;
  variant?: Extract<ButtonVariant, "secondary" | "ghost" | "danger">;
}

export const IconButton = forwardRef<HTMLButtonElement, IconButtonProps>(function IconButton(
  { label, variant = "secondary", className, type = "button", ...props },
  ref,
) {
  return <button {...props} ref={ref} type={type} aria-label={label} className={classes("cgm-icon-button", `cgm-icon-button--${variant}`, className)} />;
});

interface FieldProps {
  label: string;
  children: ReactNode;
}

export function Field({ label, children }: FieldProps) {
  return <label className="cgm-field"><span>{label}</span>{children}</label>;
}

export function Input({ className, ...props }: InputHTMLAttributes<HTMLInputElement>) {
  return <input {...props} className={classes("cgm-input", className)} />;
}

export function Select({ className, ...props }: SelectHTMLAttributes<HTMLSelectElement>) {
  return <select {...props} className={classes("cgm-select", className)} />;
}

export function Textarea({ className, ...props }: TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return <textarea {...props} className={classes("cgm-textarea", className)} />;
}

export function Badge({ children }: { children: ReactNode }) {
  return <span className="cgm-badge">{children}</span>;
}

export function Alert({ children, danger = false }: { children: ReactNode; danger?: boolean }) {
  return <div className={classes("cgm-alert", danger && "cgm-alert--danger")} role={danger ? "alert" : "status"}>{children}</div>;
}
