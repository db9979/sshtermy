# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.434"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.434/sshtermy_darwin_arm64"
      sha256 "aa1e2718792d3f79019cb623cc822cb0a284d8ad20be68f768ac9a8b3275073b"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.434/sshtermy_darwin_amd64"
      sha256 "99a4f466ab59d181481c19646c86f858733812c0d9d9e923c91a30f9ec4737d4"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.434/sshtermy_linux_arm64"
      sha256 "c48a9752ea84861baf3675baaaf1342b203da8993ab18006c11975f9ed409b43"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.434/sshtermy_linux_amd64"
      sha256 "9680a4f7a8cab1ab244e2a79de2f6e592730d549f8d89829aa2b5035b8c597f5"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
