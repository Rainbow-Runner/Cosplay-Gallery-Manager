import { useRef } from "react";
import { createPortal } from "react-dom";
import { useIntl } from "react-intl";
import { useLocation, useNavigate } from "react-router-dom";
import { useViewerIsolation } from "./useViewerIsolation";
import { viewerBackgroundLocation } from "./mediaViewerRoute";

export function MediaViewerPlaceholder({ failed = false, onClose }: {failed?: boolean; onClose?: () => void}) {
  const intl=useIntl();const location=useLocation();const navigate=useNavigate();
  const close=useRef<HTMLButtonElement>(null);useViewerIsolation(close);
  const dismiss=onClose ?? (()=>{if(viewerBackgroundLocation(location))navigate(-1);else navigate("/",{replace:true});});
  return createPortal(<div className="media-viewer__message" role="dialog" aria-modal="true" aria-label={intl.formatMessage({id:"media.viewer.title"})} onKeyDown={event=>{if(event.key==="Escape"){event.preventDefault();event.stopPropagation();dismiss();}else if(event.key==="Tab"){event.preventDefault();close.current?.focus();}}}>
    <p role={failed?"alert":"status"}>{intl.formatMessage({id:failed?"state.error":"state.loading"})}</p><button ref={close} type="button" onClick={dismiss}>{intl.formatMessage({id:"gallery.closeViewer"})}</button>
  </div>,document.body);
}
