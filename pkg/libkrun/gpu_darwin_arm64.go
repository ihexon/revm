//go:build darwin && arm64

package libkrun

/*
#cgo CFLAGS: -I ../../include
#include <libkrun.h>
#include <libkrun_display.h>
#include <stdint.h>

static int32_t revm_gpu_invalid_scanout(void *instance, uint32_t scanout_id) {
	(void)instance;
	(void)scanout_id;
	return KRUN_DISPLAY_ERR_INVALID_SCANOUT_ID;
}

static int32_t revm_gpu_destroy_display(void *instance) {
	(void)instance;
	return 0;
}

static int32_t revm_gpu_configure_scanout(void *instance,
                                           uint32_t scanout_id,
                                           uint32_t display_width,
                                           uint32_t display_height,
                                           uint32_t width,
                                           uint32_t height,
                                           uint32_t format) {
	(void)instance;
	(void)scanout_id;
	(void)display_width;
	(void)display_height;
	(void)width;
	(void)height;
	(void)format;
	return KRUN_DISPLAY_ERR_INVALID_SCANOUT_ID;
}

static int32_t revm_gpu_alloc_frame(void *instance,
                                    uint32_t scanout_id,
                                    uint8_t **buffer,
                                    size_t *buffer_size) {
	(void)instance;
	(void)scanout_id;
	(void)buffer;
	(void)buffer_size;
	return KRUN_DISPLAY_ERR_INVALID_SCANOUT_ID;
}

static int32_t revm_gpu_present_frame(void *instance,
                                      uint32_t scanout_id,
                                      uint32_t frame_id,
                                      const struct krun_rect *damage_area) {
	(void)instance;
	(void)scanout_id;
	(void)frame_id;
	(void)damage_area;
	return KRUN_DISPLAY_ERR_INVALID_SCANOUT_ID;
}

static KrunDisplayBackend revm_gpu_new_headless_display(KrunError *err_out) {
	struct krun_display_backend backend = {0};
	backend.features = KRUN_DISPLAY_FEATURE_BASIC_FRAMEBUFFER;
	backend.vtable.basic_framebuffer.destroy = revm_gpu_destroy_display;
	backend.vtable.basic_framebuffer.disable_scanout = revm_gpu_invalid_scanout;
	backend.vtable.basic_framebuffer.configure_scanout = revm_gpu_configure_scanout;
	backend.vtable.basic_framebuffer.alloc_frame = revm_gpu_alloc_frame;
	backend.vtable.basic_framebuffer.present_frame = revm_gpu_present_frame;
	return krun_display_backend_new(&backend, sizeof(backend), err_out);
}
*/
import "C"

import (
	"errors"
	"fmt"
	"linuxvm/pkg/define"
	"unsafe"
)

const venusGPUFlags = C.KRUN_VIRGL_RENDERER_FLAGS_USE_EGL |
	C.KRUN_VIRGL_RENDERER_FLAGS_VENUS |
	C.KRUN_VIRGL_RENDERER_FLAGS_RENDER_SERVER

func (v *Libkrun) setupGPU() error {
	switch v.cfg.GPUBackend {
	case "", define.GPUOff:
		return nil
	case define.GPUVenus:
		return v.setupVenusGPU()
	default:
		return fmt.Errorf("libkrun: unsupported GPU backend %q", v.cfg.GPUBackend)
	}
}

func (v *Libkrun) setupVenusGPU() error {
	var errOut C.KrunError
	backend := C.revm_gpu_new_headless_display(&errOut)
	if err := checkKrunHandle("create headless GPU display backend", unsafe.Pointer(backend), errOut); err != nil {
		return err
	}

	device := C.krun_gpu_device_new(C.uint32_t(venusGPUFlags), backend)
	if device == nil {
		C.krun_display_backend_destroy(backend)
		return errors.New("libkrun: create virtio-gpu Venus device failed")
	}
	// krun_gpu_device_new consumes the display backend on success. The MMIO
	// manager takes ownership of the GPU device below.
	C.krun_mmio_device_manager_add(v.manager, C.KrunAttachDevice(unsafe.Pointer(device)))
	return nil
}
