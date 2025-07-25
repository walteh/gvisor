load("//pkg/sync/locking:locking.bzl", "declare_mutex")
load("//tools:defs.bzl", "go_library", "go_test", "proto_library")
load("//tools/go_generics:defs.bzl", "go_template_instance")

package(default_applicable_licenses = ["//:license"])

licenses(["notice"])

declare_mutex(
    name = "virtual_filesystem_mutex",
    out = "virtual_filesystem_mutex.go",
    package = "vfs",
    prefix = "virtualFilesystem",
)

declare_mutex(
    name = "inotify_event_mutex",
    out = "inotify_event_mutex.go",
    package = "vfs",
    prefix = "inotifyEvent",
)

declare_mutex(
    name = "inotify_mutex",
    out = "inotify_mutex.go",
    package = "vfs",
    prefix = "inotify",
)

declare_mutex(
    name = "epoll_instance_mutex",
    out = "epoll_instance_mutex.go",
    package = "vfs",
    prefix = "epollReadyInstance",
)

declare_mutex(
    name = "epoll_mutex",
    out = "epoll_mutex.go",
    package = "vfs",
    prefix = "epoll",
)

go_template_instance(
    name = "epoll_interest_list",
    out = "epoll_interest_list.go",
    package = "vfs",
    prefix = "epollInterest",
    template = "//pkg/ilist:generic_list",
    types = {
        "Element": "*epollInterest",
        "Linker": "*epollInterest",
    },
)

go_template_instance(
    name = "mount_ring",
    out = "mount_ring.go",
    package = "vfs",
    prefix = "mount",
    template = "//pkg/ilist:generic_ring",
    types = {
        "Container": "*Mount",
    },
)

go_template_instance(
    name = "mount_list",
    out = "mount_list.go",
    package = "vfs",
    prefix = "follower",
    template = "//pkg/ilist:generic_list",
    types = {
        "Element": "*Mount",
        "Linker": "*Mount",
    },
)

go_template_instance(
    name = "event_list",
    out = "event_list.go",
    package = "vfs",
    prefix = "event",
    template = "//pkg/ilist:generic_list",
    types = {
        "Element": "*Event",
        "Linker": "*Event",
    },
)

go_template_instance(
    name = "file_description_refs",
    out = "file_description_refs.go",
    package = "vfs",
    prefix = "FileDescription",
    template = "//pkg/refs:refs_template",
    types = {
        "T": "FileDescription",
    },
)

go_template_instance(
    name = "mount_namespace_refs",
    out = "mount_namespace_refs.go",
    package = "vfs",
    prefix = "namespace",
    template = "//pkg/refs:refs_template",
    types = {
        "T": "MountNamespace",
    },
)

go_template_instance(
    name = "filesystem_refs",
    out = "filesystem_refs.go",
    package = "vfs",
    prefix = "Filesystem",
    template = "//pkg/refs:refs_template",
    types = {
        "T": "Filesystem",
    },
)

proto_library(
    name = "events",
    srcs = ["events.proto"],
    visibility = ["//visibility:public"],
)

go_library(
    name = "vfs",
    srcs = [
        "anonfs.go",
        "context.go",
        "debug_impl.go",
        "debug_testonly.go",
        "dentry.go",
        "device.go",
        "epoll.go",
        "epoll_instance_mutex.go",
        "epoll_interest_list.go",
        "epoll_mutex.go",
        "event_list.go",
        "file_description.go",
        "file_description_impl_util.go",
        "file_description_refs.go",
        "filesystem.go",
        "filesystem_impl_util.go",
        "filesystem_refs.go",
        "filesystem_type.go",
        "inotify.go",
        "inotify_event_mutex.go",
        "inotify_mutex.go",
        "lock.go",
        "mount.go",
        "mount_list.go",
        "mount_namespace_refs.go",
        "mount_ring.go",
        "mount_unsafe.go",
        "namespace.go",
        "opath.go",
        "options.go",
        "pathname.go",
        "permissions.go",
        "propagation.go",
        "resolving_path.go",
        "save_restore.go",
        "vfs.go",
        "virtual_filesystem_mutex.go",
    ],
    visibility = ["//pkg/sentry:internal"],
    deps = [
        ":events_go_proto",
        "//pkg/abi/linux",
        "//pkg/atomicbitops",
        "//pkg/bitmap",
        "//pkg/bits",
        "//pkg/cleanup",
        "//pkg/context",
        "//pkg/errors/linuxerr",
        "//pkg/eventchannel",
        "//pkg/fd",
        "//pkg/fdnotifier",
        "//pkg/fspath",
        "//pkg/gohacks",
        "//pkg/hostarch",
        "//pkg/log",
        "//pkg/refs",
        "//pkg/safemem",
        "//pkg/sentry/arch",
        "//pkg/sentry/fsimpl/lock",
        "//pkg/sentry/fsmetric",
        "//pkg/sentry/kernel/auth",
        "//pkg/sentry/ktime",
        "//pkg/sentry/limits",
        "//pkg/sentry/memmap",
        "//pkg/sentry/socket/unix/transport",
        "//pkg/sentry/uniqueid",
        "//pkg/sync",
        "//pkg/sync/locking",
        "//pkg/usermem",
        "//pkg/waiter",
        "@org_golang_x_sys//unix:go_default_library",
    ],
)

go_test(
    name = "vfs_test",
    size = "small",
    srcs = [
        "file_description_impl_util_test.go",
        "mount_test.go",
    ],
    library = ":vfs",
    deps = [
        "//pkg/abi/linux",
        "//pkg/atomicbitops",
        "//pkg/context",
        "//pkg/errors/linuxerr",
        "//pkg/sentry/contexttest",
        "//pkg/sync",
        "//pkg/usermem",
    ],
)
