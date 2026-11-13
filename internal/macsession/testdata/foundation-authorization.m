#import <Foundation/Foundation.h>
#import <CommonCrypto/CommonDigest.h>
static BOOL mutableKey = NO;
@interface MTDatacenterAuthInfo : NSObject <NSCoding>
@end
@implementation MTDatacenterAuthInfo
- (void)encodeWithCoder:(NSCoder *)coder {
    uint8_t key[256], digest[20];
    for (int i=0; i<256; i++) key[i]=i;
    CC_SHA1(key,256,digest);
    int64_t fingerprint; memcpy(&fingerprint,digest+12,8);
    NSData *data = mutableKey ? [NSMutableData dataWithBytes:key length:256] : [NSData dataWithBytes:key length:256];
    [coder encodeObject:data forKey:@"authKey"];
    [coder encodeInt64:fingerprint forKey:@"authKeyId"];
    [coder encodeInt32:INT32_MAX forKey:@"validUntilTimestamp"];
    [coder encodeObject:@[] forKey:@"saltSet"];
    [coder encodeObject:@{} forKey:@"authKeyAttributes"];
}
- (instancetype)initWithCoder:(NSCoder *)coder { return [super init]; }
@end
int main(int argc, const char *argv[]) { @autoreleasepool {
    mutableKey = argc > 2;
    NSDictionary *root=@{@2: [[MTDatacenterAuthInfo alloc] init]};
    NSData *archive=[NSKeyedArchiver archivedDataWithRootObject:root requiringSecureCoding:NO error:nil];
    return [archive writeToFile:[NSString stringWithUTF8String:argv[1]] atomically:YES] ? 0 : 1;
}}
