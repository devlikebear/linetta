#import <Foundation/Foundation.h>
#import <string.h>
#import <stdlib.h>

// Create a security-scoped bookmark for the directory at `path`.
// Returns malloc'd bytes and sets *out_len; returns NULL on error.
const void *linetta_bookmark_create(const char *path, size_t *out_len) {
    @autoreleasepool {
        if (path == NULL || out_len == NULL) return NULL;
        NSString *p = [NSString stringWithUTF8String:path];
        NSURL *url = [NSURL fileURLWithPath:p isDirectory:YES];
        NSError *err = nil;
        NSData *data = [url bookmarkDataWithOptions:NSURLBookmarkCreationWithSecurityScope
                    includingResourceValuesForKeys:nil
                                     relativeToURL:nil
                                             error:&err];
        if (data == nil) return NULL;
        size_t len = (size_t)[data length];
        void *buf = malloc(len);
        if (buf == NULL) return NULL;
        memcpy(buf, [data bytes], len);
        *out_len = len;
        return buf;
    }
}

// Resolve a bookmark and start security-scoped access.
// Returns a retained NSURL handle and sets *out_path (malloc'd UTF8 path).
// Returns NULL on error.
void *linetta_bookmark_start(const void *data, size_t len, char **out_path) {
    @autoreleasepool {
        if (data == NULL || out_path == NULL) return NULL;
        NSData *d = [NSData dataWithBytes:data length:len];
        BOOL stale = NO;
        NSError *err = nil;
        NSURL *url = [NSURL URLByResolvingBookmarkData:d
                                              options:NSURLBookmarkResolutionWithSecurityScope
                                        relativeToURL:nil
                                  bookmarkDataIsStale:&stale
                                                error:&err];
        if (url == nil) return NULL;
        if (![url startAccessingSecurityScopedResource]) return NULL;
        const char *fsr = [[url path] UTF8String];
        if (fsr == NULL) {
            [url stopAccessingSecurityScopedResource];
            return NULL;
        }
        char *copy = strdup(fsr);
        if (copy == NULL) {
            [url stopAccessingSecurityScopedResource];
            return NULL;
        }
        *out_path = copy;
        return (void *)CFBridgingRetain(url);
    }
}

// Stop security-scoped access and release the handle from linetta_bookmark_start.
void linetta_bookmark_stop(void *handle) {
    if (handle == NULL) return;
    NSURL *url = (NSURL *)CFBridgingRelease(handle);
    [url stopAccessingSecurityScopedResource];
}

// Free a buffer returned by create (bytes) or start (out_path).
void linetta_free(void *ptr) {
    if (ptr) free(ptr);
}
